package fleet

import "printmarshal/internal/discovery"

func discoveryCandidate(addr string) discovery.Candidate { return discovery.Candidate{Address: addr} }
