package bridge

// FooterAuthorInfo 定义首页底部作者入口的展示信息（已废弃，保留空结构体以兼容旧版前端绑定）。
type FooterAuthorInfo struct {
	ButtonText        string `json:"buttonText"`
	DialogTitle       string `json:"dialogTitle"`
	DialogContent     string `json:"dialogContent"`
	DialogConfirmText string `json:"dialogConfirmText"`
	DialogCancelText  string `json:"dialogCancelText"`
}

// GetFooterAuthorInfo 返回首页底部作者入口的展示信息（已废弃，返回空结构体）。
func (s *WindowService) GetFooterAuthorInfo() FooterAuthorInfo {
	return FooterAuthorInfo{}
}

// OpenFooterAuthorHome 打开作者主页（已废弃，不再执行任何操作）。
func (s *WindowService) OpenFooterAuthorHome() error {
	return nil
}