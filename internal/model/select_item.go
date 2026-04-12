package model

// SelectItem 选择项（用于前端下拉框、列表选择等）
type SelectItem struct {
	Value     interface{} `json:"value"`
	Label     string      `json:"label"`
	RootId    string      `json:"rootId,omitempty"`
	SubLabels []string    `json:"subLabels,omitempty"`
	LastUse   int64       `json:"lastUse"`
	ExtraData interface{} `json:"extraData,omitempty"`
}
