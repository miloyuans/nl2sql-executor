package dbresult

type Result struct {
	Columns    []string   `json:"columns"`
	Rows       [][]string `json:"rows"`
	RowCount   int        `json:"row_count"`
	Truncated  bool       `json:"truncated"`
	DurationMS int64      `json:"duration_ms"`
	Datasource string     `json:"datasource"`
	Host       string     `json:"host"`
}
