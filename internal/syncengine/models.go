package syncengine

type ProgressStage string

const (
	StagePulling   ProgressStage = "pulling"
	StageScanning  ProgressStage = "scanning"
	StageComparing ProgressStage = "comparing"
	StageApplying  ProgressStage = "applying"
	StageUploading ProgressStage = "uploading"
	StageComplete  ProgressStage = "complete"
)

type Progress struct {
	Stage            ProgressStage `json:"stage"`
	Label            string        `json:"label"`
	Percentage       int           `json:"percentage"`
	CompletedActions int           `json:"completedActions"`
	TotalActions     int           `json:"totalActions"`
	BlockedFiles     int           `json:"blockedFiles"`
	Pushed           bool          `json:"pushed"`
}
