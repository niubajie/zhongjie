package importjob

import "errors"

var (
	ErrUnsupportedType = errors.New("only docx/xlsx supported in this build")
	ErrNoRows          = errors.New("no property rows found")
	ErrFileTooLarge    = errors.New("file too large")
)

type FileInput struct {
	Name string
	Ext  string
	Data []byte
}

type Row struct {
	BuildingName string
	AreaSqm      float64
	RentYuan     float64
	Layout       string
	Title        string
}

type Result struct {
	JobUUID     string `json:"jobId"`
	TotalRows   int    `json:"totalRows"`
	SuccessRows int    `json:"successRows"`
	ErrorRows   int    `json:"errorRows"`
}
