package notify

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"housing/internal/secure"
)

type fakeStore struct {
	records      []Record
	openids      map[int64]string
	sent         []int64
	retried      map[int64]string
	terminals    map[int64]string
	retryStatus  string
	retriedCalls int
	audits       []string
	lastStatus   string
}

func (f *fakeStore) Due(context.Context, int) ([]Record, error) { return f.records, nil }

func (f *fakeStore) OpenID(_ context.Context, userID int64) (string, error) {
	return f.openids[userID], nil
}

func (f *fakeStore) MarkSent(_ context.Context, id int64) error {
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeStore) MarkRetry(_ context.Context, id int64, message string, _ int) (string, error) {
	if f.retried == nil {
		f.retried = map[int64]string{}
	}
	f.retried[id] = message
	if f.retryStatus == "" {
		return StatusRetrying, nil
	}
	return f.retryStatus, nil
}

func (f *fakeStore) MarkTerminal(_ context.Context, id int64, status, message string) error {
	if f.terminals == nil {
		f.terminals = map[int64]string{}
	}
	f.terminals[id] = status + ": " + message
	return nil
}

func (f *fakeStore) List(_ context.Context, status string, _ int) ([]OutboxRecord, error) {
	if status != "" && !validOutboxStatuses[status] {
		return nil, ErrInvalidOutboxQuery
	}
	f.lastStatus = status
	return []OutboxRecord{{ID: 1, Status: StatusSent}}, nil
}

func (f *fakeStore) Retry(_ context.Context, id int64) error {
	f.retriedCalls++
	if id == 404 {
		return ErrOutboxNotFound
	}
	return nil
}

func (f *fakeStore) Audit(_ context.Context, _ int64, action, _, _ string) int64 {
	f.audits = append(f.audits, action)
	return 1
}

type fakeSender struct {
	configured bool
	err        error
	calls      int
}

func (s *fakeSender) Configured() bool { return s.configured }

func (s *fakeSender) SendSubscribeMessage(context.Context, string, string, string, map[string]string) error {
	s.calls++
	return s.err
}

func testNotifyPII(t *testing.T) *secure.PII {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	p, err := secure.NewPII(key, "notify-hash-key")
	if err != nil {
		t.Fatalf("NewPII: %v", err)
	}
	return p
}

func encryptedPayload(t *testing.T, pii *secure.PII) []byte {
	t.Helper()
	enc, err := pii.EncryptString(`{"reminder":"租客联系提醒","title":"房源提醒","district":"西湖区","street":"文三路"}`)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	return enc
}

func newNotifyService(t *testing.T, store *fakeStore, sender *fakeSender, maxAttempts int) *Service {
	t.Helper()
	return NewService(store, testNotifyPII(t), sender, "tpl-1", "pages/map/index", "", maxAttempts)
}

func TestProcessNotConfigured(t *testing.T) {
	store := &fakeStore{records: []Record{{ID: 1, RecipientID: 7}}}
	sender := &fakeSender{configured: false}
	result, err := newNotifyService(t, store, sender, 0).Process(context.Background(), 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.NotConfigured != 1 || sender.calls != 0 {
		t.Fatalf("未配置时不应发送: %+v calls=%d", result, sender.calls)
	}
	if store.terminals[1] != StatusNotConfigured+": WECHAT_SUBSCRIBE_TEMPLATE_ID or WeChat credentials not configured" {
		t.Fatalf("应标记 not_configured: %v", store.terminals)
	}
}

func TestProcessSendsAndMarksSent(t *testing.T) {
	store := &fakeStore{
		records: []Record{{ID: 2, RecipientID: 7, PayloadEnc: encryptedPayload(t, testNotifyPII(t))}},
		openids: map[int64]string{7: "openid-7"},
	}
	sender := &fakeSender{configured: true}
	result, err := newNotifyService(t, store, sender, 0).Process(context.Background(), 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Sent != 1 || sender.calls != 1 || len(store.sent) != 1 {
		t.Fatalf("应发送成功: %+v calls=%d sent=%v", result, sender.calls, store.sent)
	}
}

func TestProcessRetriesThenMarksDead(t *testing.T) {
	payload := encryptedPayload(t, testNotifyPII(t))
	openids := map[int64]string{7: "openid-7"}

	store := &fakeStore{records: []Record{{ID: 3, RecipientID: 7, PayloadEnc: payload}}, openids: openids}
	result, err := newNotifyService(t, store, &fakeSender{configured: true, err: errors.New("errcode 45009")}, 5).Process(context.Background(), 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Retrying != 1 || store.retried[3] != "errcode 45009" {
		t.Fatalf("失败应进入重试: %+v retried=%v", result, store.retried)
	}

	deadStore := &fakeStore{
		records:     []Record{{ID: 4, RecipientID: 7, PayloadEnc: encryptedPayload(t, testNotifyPII(t))}},
		openids:     openids,
		retryStatus: StatusFailed,
	}
	result, err = newNotifyService(t, deadStore, &fakeSender{configured: true, err: errors.New("errcode 40003")}, 1).Process(context.Background(), 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Dead != 1 || result.Retrying != 0 {
		t.Fatalf("达到上限应转 failed: %+v", result)
	}
}

func TestProcessMarksDeadWhenOpenIDMissing(t *testing.T) {
	store := &fakeStore{
		records: []Record{{ID: 5, RecipientID: 8, PayloadEnc: encryptedPayload(t, testNotifyPII(t))}},
		openids: map[int64]string{},
	}
	sender := &fakeSender{configured: true}
	result, err := newNotifyService(t, store, sender, 0).Process(context.Background(), 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Dead != 1 || sender.calls != 0 {
		t.Fatalf("缺少 openid 不应发送: %+v calls=%d", result, sender.calls)
	}
}

func TestListOutboxValidatesStatus(t *testing.T) {
	svc := newNotifyService(t, &fakeStore{}, &fakeSender{}, 0)
	if _, err := svc.ListOutbox(context.Background(), "bogus", 10); !errors.Is(err, ErrInvalidOutboxQuery) {
		t.Fatalf("非法状态应报错: %v", err)
	}
	list, err := svc.ListOutbox(context.Background(), StatusSent, 10)
	if err != nil || len(list.Items) != 1 || list.Items[0].Status != StatusSent {
		t.Fatalf("查询失败: %+v %v", list, err)
	}
}

func TestRetryOutboxWritesAudit(t *testing.T) {
	store := &fakeStore{}
	svc := newNotifyService(t, store, &fakeSender{}, 0)
	if err := svc.RetryOutbox(context.Background(), 1, 9); err != nil {
		t.Fatalf("RetryOutbox: %v", err)
	}
	if store.retriedCalls != 1 || len(store.audits) != 1 || store.audits[0] != "prospect.notification_outbox.retry" {
		t.Fatalf("重试应写审计: calls=%d audits=%v", store.retriedCalls, store.audits)
	}
	if err := svc.RetryOutbox(context.Background(), 1, 404); !errors.Is(err, ErrOutboxNotFound) {
		t.Fatalf("不存在应报错: %v", err)
	}
	if len(store.audits) != 1 {
		t.Fatalf("失败重试不应写审计: %v", store.audits)
	}
}
