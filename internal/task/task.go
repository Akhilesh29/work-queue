package task

type Task struct {
	JobID    string                 `json:"job_id,omitempty"`
	Type     string                 `json:"type"`
	Payload  map[string]interface{} `json:"payload"`
	Retries  int                    `json:"retries"`
	Attempts int                    `json:"attempts,omitempty"`
}
