package run

type Handler struct {
}

func (*Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

}

func (*Handler) serveStatus(w http.ResponseWriter, r *http.Request) {

}

func (*Handler) SetNodeStatus() error {

}
