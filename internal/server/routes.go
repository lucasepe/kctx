package server

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.healthz)
	s.mux.HandleFunc("/version", s.version)
	s.mux.HandleFunc("/context/pod/", s.contextPod)
	s.mux.HandleFunc("/graph/pod/", s.graphPod)
	s.mux.HandleFunc("/trace/service/", s.traceService)
	s.mux.HandleFunc("/health/namespace/", s.healthNamespace)
	s.mux.HandleFunc("/dump/namespace/", s.dumpNamespace)
}
