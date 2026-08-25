package telemetry

// Sample mirrors the daemon's telemetry output exactly. It is the wire format
// for `mtroamd stats` and is mirrored by hub/internal/api/types.go.
type Sample struct {
	TS        int64          `json:"ts"`
	Host      string         `json:"host"`
	UptimeSec uint64         `json:"uptime_s"`
	Load      [3]float64     `json:"load"`
	CPUPct    float64        `json:"cpu_pct"`
	CPUPer    []float64      `json:"cpu_per,omitempty"`
	Mem       MemStat        `json:"mem"`
	Swap      MemStat        `json:"swap,omitempty"`
	Disks     []DiskStat     `json:"disks,omitempty"`
	Net       []NetStat      `json:"net,omitempty"`
	Temps     []TempStat     `json:"temps,omitempty"`
	Sessions  SessionStat    `json:"sessions"`
	Extra     map[string]any `json:"extra,omitempty"`
}

type MemStat struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
	Free  uint64 `json:"free"`
}

type DiskStat struct {
	Mount string `json:"mount"`
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
	Free  uint64 `json:"free"`
}

type NetStat struct {
	Name    string `json:"name"`
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

type TempStat struct {
	Name  string  `json:"name"`
	TempC float64 `json:"temp_c"`
}

type SessionStat struct {
	Total    int `json:"total"`
	Attached int `json:"attached"`
	Agent    int `json:"agent"`
}
