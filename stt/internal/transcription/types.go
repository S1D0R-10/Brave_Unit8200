package transcription

import "time"

type Segment struct {
	Index   int    `json:"index"`
	StartMS int64  `json:"startMs"`
	EndMS   int64  `json:"endMs"`
	Text    string `json:"text"`
}

type SourceInfo struct {
	FileName   string `json:"fileName"`
	SizeBytes  int64  `json:"sizeBytes"`
	DurationMS int64  `json:"durationMs"`
}

type EngineInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Model   string `json:"model"`
}

type ProcessingInfo struct {
	WallTimeMS     int64   `json:"wallTimeMs"`
	RealtimeFactor float64 `json:"realtimeFactor"`
}

type Result struct {
	SchemaVersion   string         `json:"schemaVersion"`
	TranscriptionID string         `json:"transcriptionId"`
	Source          SourceInfo     `json:"source"`
	Language        string         `json:"language"`
	Text            string         `json:"text"`
	Segments        []Segment      `json:"segments"`
	Engine          EngineInfo     `json:"engine"`
	Processing      ProcessingInfo `json:"processing"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type ArtifactPaths struct {
	TXT string `json:"txt"`
}
