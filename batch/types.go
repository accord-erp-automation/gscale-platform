package batch

import "time"

const (
	VendorID  = 0x195F
	ProductID = 0x0001

	DefaultQRBaseURL        = "https://scan.wspace.sbs/L/"
	DefaultArchiveQRBaseURL = "https://scan.wspace.sbs/A/"

	TextGraphicName = "TEXTLBL"
	QRGraphicName   = "QRLBL"

	DefaultNotoSansRegular = "/usr/share/fonts/noto/NotoSans-Regular.ttf"
	DefaultNotoSansBold    = "/usr/share/fonts/noto/NotoSans-Bold.ttf"
)

var RecoverySequence = []string{
	"~S,ESG",
	"^AD",
	"^XSET,IMMEDIATE,1",
	"^XSET,ACTIVERESPONSE,1",
	"~Z",
	"~S,CANCEL",
	"~S,SENSOR",
}

type LabelOptions struct {
	LabelLengthMM int
	LabelGapMM    int
	LabelWidthMM  int
	DPI           int
	SafeMarginMM  float64
	QRBoxMM       float64
	QRMode        string
	RegularFont   string
	BoldFont      string
}

func DefaultPackLabelOptions() LabelOptions {
	return LabelOptions{
		LabelLengthMM: 50,
		LabelGapMM:    3,
		LabelWidthMM:  50,
		DPI:           203,
		SafeMarginMM:  4.0,
		QRBoxMM:       18.0,
		QRMode:        "url",
		RegularFont:   DefaultNotoSansRegular,
		BoldFont:      DefaultNotoSansBold,
	}
}

func DefaultSimpleLabelOptions() LabelOptions {
	return LabelOptions{
		LabelLengthMM: 25,
		LabelGapMM:    3,
		LabelWidthMM:  50,
		DPI:           203,
		QRBoxMM:       35,
		RegularFont:   DefaultNotoSansRegular,
		BoldFont:      DefaultNotoSansBold,
	}
}

func DefaultArchiveLabelOptions() LabelOptions {
	return LabelOptions{
		LabelLengthMM: 80,
		LabelGapMM:    3,
		LabelWidthMM:  60,
		DPI:           203,
		SafeMarginMM:  4.0,
		QRBoxMM:       14.0,
		RegularFont:   DefaultNotoSansRegular,
		BoldFont:      DefaultNotoSansBold,
	}
}

type PackLabel struct {
	CompanyName string
	ProductName string
	KGText      string
	BruttoText  string
	EPC         string
}

type ArchiveBatchLabel struct {
	SessionID string
	ItemName  string
	QtyText   string
	BatchTime string
}

type ArchiveBatchData struct {
	Commands       []string
	TextGraphicBMP []byte
	QRGraphicBMP   []byte
	QRPayload      string
}

type PackLabelData struct {
	Commands       []string
	TextGraphicBMP []byte
	QRGraphicBMP   []byte
	QRPayload      string
}

type Transport interface {
	Send(command string, read bool, pause time.Duration) (string, error)
	WriteRaw(payload []byte) error
	Close() error
}
