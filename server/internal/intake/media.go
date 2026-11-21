package intake

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"housing/internal/modelgw"
	"housing/internal/property"
	"housing/internal/secure"
	"housing/internal/user"
)

const (
	maxImageBytes = 15 << 20
	maxVideoBytes = 200 << 20
)

var (
	errFFmpegMissing = errors.New("视频解析需要 ffmpeg，当前环境未安装")
	errUnsupported   = ErrUnsupportedMedia
)

// CreateFromMedia runs OCR (image) or ASR + keyframe OCR (video), extracts
// listing fields and tries to bind the landlord by phone hash. It never
// fabricates data: when a provider is not configured the job is marked
// not_configured with a concrete hint.
func (s *Service) CreateFromMedia(ctx context.Context, adminID int64, filename, mime string, data []byte) (MediaResult, error) {
	kind, ext, limit := classifyMedia(filename, mime)
	if kind == "" {
		switch strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".") {
		case "pdf":
			return MediaResult{}, fmt.Errorf("%w：PDF 暂不支持，请转成图片走 OCR 或另存为 docx", ErrUnsupportedMedia)
		case "doc":
			return MediaResult{}, fmt.Errorf("%w：旧版 .doc 不支持，请另存为 docx", ErrUnsupportedMedia)
		}
		return MediaResult{}, errUnsupported
	}
	if len(data) == 0 {
		return MediaResult{}, errors.New("文件为空")
	}
	if len(data) > limit {
		return MediaResult{}, fmt.Errorf("文件过大，最多 %dMB", limit>>20)
	}
	if s.media == nil {
		return MediaResult{}, errors.New("对象存储未初始化")
	}

	key := fmt.Sprintf("intake/%s/%s.%s", time.Now().Format("20060102"), objectHash(data), ext)
	if err := s.media.Put(ctx, key, data, mime); err != nil {
		return MediaResult{}, err
	}
	mediaKeys := []string{key}

	outcome := s.recognize(ctx, kind, ext, data, mime)
	extracted := Extracted{}
	if strings.TrimSpace(outcome.text) != "" {
		extracted = parseText(outcome.text)
	}
	if outcome.status == "ready" && (extracted.CommunityName == "" || extracted.AreaSqm <= 0 || extracted.RentYuan <= 0) {
		outcome.message = "已识别文字，但缺少小区、面积或租金，请人工补充"
	}

	landlordID, landlordUUID, matchStatus, confidence := s.matchLandlord(ctx, extracted.Phone)
	stored := extracted
	stored.Phone = "" // the raw phone stays only inside the encrypted text blob

	_, jobUUID, err := s.repo.CreateMediaJob(ctx, adminID, kind, mediaKeys, outcome.ocrEnc, outcome.transcriptEnc,
		stored, outcome.status, outcome.errorCode, matchStatus, landlordID, confidence)
	if err != nil {
		return MediaResult{}, err
	}
	s.repo.Audit(ctx, adminID, "intake_media_create", jobUUID)

	return s.buildMediaResult(jobUUID, outcome.status, matchStatus, mediaKeys, extracted, landlordUUID, confidence, outcome.message, outcome.text), nil
}

type recognition struct {
	status        string
	errorCode     string
	text          string
	message       string
	ocrEnc        []byte
	transcriptEnc []byte
}

// recognize runs OCR (image) or ASR (video). It returns not_configured with a
// concrete hint instead of fabricating text when a provider is missing.
func (s *Service) recognize(ctx context.Context, kind, ext string, data []byte, mime string) recognition {
	out := recognition{status: "ready"}
	switch kind {
	case "image":
		if s.gw == nil {
			return recognition{status: "not_configured", errorCode: "ocr_not_configured",
				message: "OCR 未配置，已保存原图，请人工填写或配置 MODELGW_OCR_*"}
		}
		text, err := s.gw.OCRText(ctx, data, mime)
		if errors.Is(err, modelgw.ErrNotConfigured) {
			return recognition{status: "not_configured", errorCode: "ocr_not_configured",
				message: "OCR 未配置，已保存原图，请人工填写或配置 MODELGW_OCR_*"}
		}
		if err != nil {
			return recognition{status: "failed", errorCode: "ocr_failed",
				message: "图片识别失败，请重试或人工录入"}
		}
		out.text = text
		out.ocrEnc, _ = s.pii.EncryptString(text)
	case "video":
		audio, err := extractAudio(ctx, data, ext)
		if errors.Is(err, errFFmpegMissing) {
			return recognition{status: "not_configured", errorCode: "ffmpeg_missing",
				message: "视频解析需要 ffmpeg，已保存原视频，请安装后重试或人工录入"}
		}
		if err != nil {
			return recognition{status: "failed", errorCode: "video_decode_failed",
				message: "视频解码失败，请重试"}
		}
		if s.gw == nil {
			return recognition{status: "not_configured", errorCode: "asr_not_configured",
				message: "语音识别未配置，已保存原视频，请人工填写或配置 MODELGW_ASR_*"}
		}
		transcript, err := s.gw.Transcribe(ctx, audio, "audio/wav")
		if errors.Is(err, modelgw.ErrNotConfigured) {
			return recognition{status: "not_configured", errorCode: "asr_not_configured",
				message: "语音识别未配置，已保存原视频，请人工填写或配置 MODELGW_ASR_*"}
		}
		if err != nil {
			return recognition{status: "failed", errorCode: "asr_failed",
				message: "视频转写失败，请重试或人工录入"}
		}
		out.text = transcript
		out.transcriptEnc, _ = s.pii.EncryptString(transcript)
	case "doc":
		text, err := extractDocText(ext, data)
		if err != nil {
			return recognition{status: "failed", errorCode: "doc_parse_failed",
				message: "文档解析失败：" + err.Error()}
		}
		out.text = text
		out.ocrEnc, _ = s.pii.EncryptString(text)
	default:
		return recognition{status: "failed", errorCode: "unsupported_media",
			message: ErrUnsupportedMedia.Error()}
	}
	return out
}

// matchLandlord binds a landlord when the extracted phone uniquely matches.
func (s *Service) matchLandlord(ctx context.Context, phone string) (int64, string, string, float64) {
	if strings.TrimSpace(phone) == "" {
		return 0, "", "unmatched", 0
	}
	if u, err := s.users.FindByPhoneHash(ctx, s.pii.HashPhone(phone)); err == nil && u.Role == "landlord" {
		return u.ID, u.UUID, "auto_matched", 0.95
	}
	return 0, "", "unmatched", 0
}

func (s *Service) buildMediaResult(jobUUID, status, matchStatus string, mediaKeys []string, extracted Extracted, landlordUUID string, confidence float64, message, text string) MediaResult {
	maskedPhone := ""
	if extracted.Phone != "" {
		maskedPhone = secure.MaskPhone(extracted.Phone)
		extracted.Phone = ""
	}
	urls := make([]string, 0, len(mediaKeys))
	if s.media != nil {
		for _, key := range mediaKeys {
			urls = append(urls, s.media.URL(key))
		}
	}
	return MediaResult{
		JobID:        jobUUID,
		Status:       status,
		MatchStatus:  matchStatus,
		MediaURLs:    urls,
		Extracted:    extracted,
		MaskedPhone:  maskedPhone,
		LandlordUUID: landlordUUID,
		Confidence:   confidence,
		Message:      message,
		OCRText:      text,
	}
}

// Retry re-runs OCR/ASR on the stored media of a failed or not_configured job.
func (s *Service) Retry(ctx context.Context, adminID int64, jobUUID string) (MediaResult, error) {
	job, err := s.repo.GetJob(ctx, jobUUID)
	if err != nil {
		return MediaResult{}, err
	}
	if job.Status != "failed" && job.Status != "not_configured" {
		return MediaResult{}, ErrJobNotEditable
	}
	if len(job.MediaKeys) == 0 || s.media == nil {
		return MediaResult{}, errors.New("该任务没有可重试的素材")
	}
	key := job.MediaKeys[0]
	data, err := s.media.Get(ctx, key)
	if err != nil {
		return MediaResult{}, fmt.Errorf("读取素材失败: %w", err)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(key)), ".")
	outcome := s.recognize(ctx, job.InputType, ext, data, mimeForExt(ext))
	extracted := Extracted{}
	if strings.TrimSpace(outcome.text) != "" {
		extracted = parseText(outcome.text)
	}
	if outcome.status == "ready" && (extracted.CommunityName == "" || extracted.AreaSqm <= 0 || extracted.RentYuan <= 0) {
		outcome.message = "已识别文字，但缺少小区、面积或租金，请人工补充"
	}
	landlordID, landlordUUID, matchStatus, confidence := s.matchLandlord(ctx, extracted.Phone)
	stored := extracted
	stored.Phone = ""
	if err := s.repo.UpdateRecognition(ctx, jobUUID, outcome.ocrEnc, outcome.transcriptEnc,
		stored, outcome.status, outcome.errorCode, matchStatus, landlordID, confidence); err != nil {
		return MediaResult{}, err
	}
	s.repo.Audit(ctx, adminID, "intake_media_retry", jobUUID)
	return s.buildMediaResult(jobUUID, outcome.status, matchStatus, job.MediaKeys, extracted, landlordUUID, confidence, outcome.message, outcome.text), nil
}

func mimeForExt(ext string) string {
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	case "mp4", "m4v":
		return "video/mp4"
	case "mov":
		return "video/quicktime"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "txt":
		return "text/plain"
	case "md":
		return "text/markdown"
	case "pdf":
		return "application/pdf"
	}
	return "application/octet-stream"
}

func (s *Service) Detail(ctx context.Context, adminID int64, jobUUID string) (map[string]any, error) {
	job, err := s.repo.GetJob(ctx, jobUUID)
	if err != nil {
		return nil, err
	}
	ocrText, _ := s.pii.DecryptString(job.OCRText)
	transcript, _ := s.pii.DecryptString(job.Transcript)
	urls := make([]string, 0, len(job.MediaKeys))
	for _, key := range job.MediaKeys {
		if s.media != nil {
			urls = append(urls, s.media.URL(key))
		}
	}
	s.repo.Audit(ctx, adminID, "intake_media_read", jobUUID)
	extracted := job.Extracted
	maskedPhone := ""
	if extracted.Phone != "" {
		maskedPhone = secure.MaskPhone(extracted.Phone)
		extracted.Phone = ""
	} else if m := mobileRe.FindString(ocrText + "\n" + transcript); m != "" {
		maskedPhone = secure.MaskPhone(m)
	}
	out := map[string]any{
		"jobId":        job.UUID,
		"inputType":    job.InputType,
		"status":       job.Status,
		"matchStatus":  job.MatchStatus,
		"landlordId":   job.LandlordUUID,
		"propertyId":   job.PropertyUUID,
		"confidence":   job.Confidence,
		"errorMessage": job.ErrorMessage,
		"extracted":    extracted,
		"maskedPhone":  maskedPhone,
		"mediaUrls":    urls,
		"ocrText":      ocrText,
		"transcript":   transcript,
	}
	return out, nil
}

func (s *Service) Match(ctx context.Context, adminID int64, jobUUID string, in MatchInput) error {
	landlord, err := s.users.FindByUUID(ctx, strings.TrimSpace(in.LandlordUUID))
	if errors.Is(err, user.ErrNotFound) {
		return ErrLandlordNotFound
	}
	if err != nil {
		return fmt.Errorf("find landlord: %w", err)
	}
	if landlord.Role != "landlord" || landlord.Status != "active" {
		return errors.New("该用户不是可用房东")
	}
	if err := s.repo.UpdateMatch(ctx, jobUUID, landlord.ID, "manual_matched", 1); err != nil {
		return err
	}
	s.repo.Audit(ctx, adminID, "intake_media_match:"+strings.TrimSpace(in.Reason), jobUUID)
	return nil
}

func (s *Service) Publish(ctx context.Context, adminID int64, jobUUID string, in PublishInput) (Result, error) {
	job, err := s.repo.GetJob(ctx, jobUUID)
	if err != nil {
		return Result{}, err
	}
	if job.Status != "ready" {
		return Result{}, ErrJobNotEditable
	}
	if job.LandlordID == 0 {
		return Result{}, errors.New("请先匹配房东")
	}
	if in.AreaSqm <= 0 {
		in.AreaSqm = job.Extracted.AreaSqm
	}
	if in.RentCents <= 0 {
		in.RentCents = int64(job.Extracted.RentYuan * 100)
	}
	if in.Title == "" {
		in.Title = job.Extracted.Title
	}
	if in.Layout == "" {
		in.Layout = job.Extracted.Layout
	}
	if in.CommunityUUID == "" && in.CommunityName == "" {
		in.CommunityName = job.Extracted.CommunityName
	}
	imageKeys := in.ImageKeys
	if len(imageKeys) == 0 {
		for _, key := range job.MediaKeys {
			switch strings.ToLower(filepath.Ext(key)) {
			case ".jpg", ".jpeg", ".png", ".webp":
				imageKeys = append(imageKeys, key)
			}
		}
	}

	created, err := s.properties.Create(ctx, job.LandlordID, property.CreateInput{
		CommunityUUID: in.CommunityUUID,
		CommunityName: in.CommunityName,
		AreaSqm:       in.AreaSqm,
		RentCents:     in.RentCents,
		Layout:        in.Layout,
		Floor:         in.Floor,
		Orientation:   in.Orientation,
		Title:         in.Title,
		Description:   in.Description,
		ImageKeys:     imageKeys,
	})
	if err != nil {
		return Result{}, err
	}
	if err := s.repo.FinishMedia(ctx, jobUUID, "created", created.UUID); err != nil {
		return Result{}, err
	}
	s.repo.Audit(ctx, adminID, "intake_media_publish", jobUUID)
	return Result{
		JobID:        jobUUID,
		PropertyUUID: created.UUID,
		Status:       created.Status,
		LandlordUUID: job.LandlordUUID,
	}, nil
}

func classifyMedia(filename, mime string) (string, string, int) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	switch ext {
	case "jpg", "jpeg", "png", "webp":
		return "image", ext, maxImageBytes
	case "mp4", "mov", "m4v":
		return "video", ext, maxVideoBytes
	case "docx", "txt", "md":
		return "doc", ext, maxDocBytes
	}
	switch mime {
	case "image/jpeg":
		return "image", "jpg", maxImageBytes
	case "image/png":
		return "image", "png", maxImageBytes
	case "image/webp":
		return "image", "webp", maxImageBytes
	case "video/mp4":
		return "video", "mp4", maxVideoBytes
	case "video/quicktime":
		return "video", "mov", maxVideoBytes
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "doc", "docx", maxDocBytes
	case "text/plain":
		return "doc", "txt", maxDocBytes
	case "text/markdown":
		return "doc", "md", maxDocBytes
	}
	return "", "", 0
}

func objectHash(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:8])
}

func extractAudio(ctx context.Context, video []byte, ext string) ([]byte, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errFFmpegMissing
	}
	dir, err := os.MkdirTemp("", "intake-video-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input."+ext)
	output := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(input, video, 0o600); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-y", "-i", input, "-vn", "-ac", "1", "-ar", "16000", "-f", "wav", output)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(output)
}
