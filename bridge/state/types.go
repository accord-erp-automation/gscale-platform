package state

type Snapshot struct {
	Scale        ScaleSnapshot        `json:"scale"`
	Zebra        ZebraSnapshot        `json:"zebra"`
	Batch        BatchSnapshot        `json:"batch"`
	PrintRequest PrintRequestSnapshot `json:"print_request"`
	UpdatedAt    string               `json:"updated_at,omitempty"`
}

type ScaleSnapshot struct {
	Source    string   `json:"source,omitempty"`
	Port      string   `json:"port,omitempty"`
	Weight    *float64 `json:"weight"`
	Unit      string   `json:"unit,omitempty"`
	Stable    *bool    `json:"stable"`
	Error     string   `json:"error,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type ZebraSnapshot struct {
	Connected   bool   `json:"connected"`
	DevicePath  string `json:"device_path,omitempty"`
	Name        string `json:"name,omitempty"`
	DeviceState string `json:"device_state,omitempty"`
	MediaState  string `json:"media_state,omitempty"`
	ReadLine1   string `json:"read_line1,omitempty"`
	ReadLine2   string `json:"read_line2,omitempty"`
	LastEPC     string `json:"last_epc,omitempty"`
	Verify      string `json:"verify,omitempty"`
	Action      string `json:"action,omitempty"`
	Error       string `json:"error,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type BatchSnapshot struct {
	Active         bool    `json:"active"`
	ChatID         int64   `json:"chat_id,omitempty"`
	ItemCode       string  `json:"item_code,omitempty"`
	ItemName       string  `json:"item_name,omitempty"`
	Warehouse      string  `json:"warehouse,omitempty"`
	PrintMode      string  `json:"print_mode,omitempty"`
	Printer        string  `json:"printer,omitempty"`
	QuantitySource string  `json:"quantity_source,omitempty"`
	ManualQtyKG    float64 `json:"manual_qty_kg,omitempty"`
	Tare           bool    `json:"tare,omitempty"`
	TareKG         float64 `json:"tare_kg,omitempty"`
	TotalQty       float64 `json:"total_qty,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
}

type PrintRequestSnapshot struct {
	EPC         string   `json:"epc,omitempty"`
	Qty         *float64 `json:"qty"`
	GrossQty    *float64 `json:"gross_qty,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	ItemCode    string   `json:"item_code,omitempty"`
	ItemName    string   `json:"item_name,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Printer     string   `json:"printer,omitempty"`
	Tare        bool     `json:"tare,omitempty"`
	TareKG      float64  `json:"tare_kg,omitempty"`
	Status      string   `json:"status,omitempty"`
	Error       string   `json:"error,omitempty"`
	RequestedAt string   `json:"requested_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}
