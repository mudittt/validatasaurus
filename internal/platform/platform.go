package platform

type SQLFile struct {
	Name    string
	Content []byte
}

type Platform interface {
	Name() string
	FetchSQLFiles(ticketURL string) ([]SQLFile, error)
	PostComment(ticketURL, body string) error
}
