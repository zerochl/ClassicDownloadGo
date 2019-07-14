package request

type Request struct {
	Method   string
	URL      string
	Header   map[string]string
	Content  []byte
	FileName string
	FileMD5  string
}

type InitRequest struct {
	DownloadPath string `json:"download_path"`
	MinSplitBurst int64 `json:"min_split_burst"`
}