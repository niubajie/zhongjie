package importjob

import (
	"context"
	"strings"

	"housing/internal/building"
	"housing/internal/property"
)

type Service struct {
	repo       *Repository
	properties *property.Service
	buildings  *building.Service
}

func NewService(repo *Repository, properties *property.Service, buildings *building.Service) *Service {
	return &Service{repo: repo, properties: properties, buildings: buildings}
}

func (s *Service) Import(ctx context.Context, landlordID int64, in FileInput) (Result, error) {
	if len(in.Data) > 20<<20 {
		return Result{}, ErrFileTooLarge
	}
	ext := strings.TrimPrefix(strings.ToLower(in.Ext), ".")
	jobID, jobUUID, err := s.repo.CreateJob(ctx, landlordID, in.Name, ext)
	if err != nil {
		return Result{}, err
	}

	rows, err := Parse(in)
	if err != nil {
		_ = s.repo.FinishJob(ctx, jobID, 0, 0, 0, err.Error())
		return Result{JobUUID: jobUUID}, err
	}

	success, failed := 0, 0
	for i, row := range rows {
		b, err := s.buildings.FindByName(ctx, "440300", row.BuildingName)
		if err != nil {
			failed++
			_ = s.repo.AddDraft(ctx, jobID, i+1, row, "error", "楼宇未匹配: "+row.BuildingName, "")
			continue
		}
		created, err := s.properties.Create(ctx, landlordID, property.CreateInput{
			BuildingUUID: b.UUID,
			AreaSqm:      row.AreaSqm,
			RentCents:    int64(row.RentYuan * 100),
			Layout:       row.Layout,
			Title:        row.Title,
			Description:  "从文档导入",
		})
		if err != nil {
			failed++
			_ = s.repo.AddDraft(ctx, jobID, i+1, row, "error", err.Error(), "")
			continue
		}
		success++
		_ = s.repo.AddDraft(ctx, jobID, i+1, row, "created", "", created.UUID)
	}
	if err := s.repo.FinishJob(ctx, jobID, len(rows), success, failed, ""); err != nil {
		return Result{}, err
	}
	return Result{JobUUID: jobUUID, TotalRows: len(rows), SuccessRows: success, ErrorRows: failed}, nil
}
