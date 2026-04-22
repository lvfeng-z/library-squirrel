package dto

// SelectItem 选择项（用于前端下拉框、列表选择等）
type SelectItem struct {
	Value     any      `json:"value"`
	Label     string   `json:"label"`
	RootId    string   `json:"rootId,omitempty"`
	SubLabels []string `json:"subLabels,omitempty"`
	ExtraData any      `json:"extraData,omitempty"`
}
