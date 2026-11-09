package option

import (
	"reflect"
	"strconv"
	"strings"

	xbadoption "github.com/sagernet/sing-box/common/xray/json/badoption"
	"github.com/sagernet/sing-box/schema"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json"
)

// FinalMaskOptions — блок streamSettings.finalmask из Xray, принимаемый как
// есть (поля масок в camelCase, как у Xray и у форка Hiddify). Из масок
// реализована только tcp-маска fragment; любая другая маска отвергается при
// разборе, а не пропускается молча: сервер, ждущий маску, без неё не пустит.
type FinalMaskOptions struct {
	TCP []FinalMask `json:"tcp,omitempty"`
	UDP []FinalMask `json:"udp,omitempty"`
	// QuicParams принимается, чтобы блок из Xray грузился целиком, и не
	// используется: TCP-маскам параметры QUIC не нужны.
	QuicParams FinalMaskQuicParams `json:"quicParams,omitempty"`
}

func (o *FinalMaskOptions) UnmarshalJSON(content []byte) error {
	type plain FinalMaskOptions
	if err := json.UnmarshalDisallowUnknownFields(content, (*plain)(o)); err != nil {
		return err
	}
	if len(o.UDP) > 0 {
		return E.New("finalmask: udp masks are not supported")
	}
	return nil
}

// udp описан тем же списком масок; что udp-маски не поддержаны, проверяет
// UnmarshalJSON (в схеме нет maxItems).
func (FinalMaskOptions) DescribeSchema(builder schema.Builder) (*schema.Node, error) {
	return builder.Define("FinalMaskOptions", func() (*schema.Node, error) {
		mask, err := builder.Describe(reflect.TypeFor[FinalMask]())
		if err != nil {
			return nil, err
		}
		object := schema.StrictObject()
		object.Properties.Put("tcp", &schema.Node{Type: "array", Items: mask})
		object.Properties.Put("udp", &schema.Node{Type: "array", Items: mask})
		object.Properties.Put("quicParams", schema.LooseObject())
		return object, nil
	})
}

type FinalMaskQuicParams json.RawMessage

func (p FinalMaskQuicParams) MarshalJSON() ([]byte, error) {
	if len(p) == 0 {
		return []byte("null"), nil
	}
	return p, nil
}

func (p *FinalMaskQuicParams) UnmarshalJSON(content []byte) error {
	*p = append((*p)[:0], content...)
	return nil
}

func (FinalMaskQuicParams) DescribeSchema(schema.Builder) (*schema.Node, error) {
	return schema.LooseObject(), nil
}

const FinalMaskTypeFragment = "fragment"

type FinalMask struct {
	Type     string
	Fragment FragmentMaskOptions
}

type rawFinalMask struct {
	Type     string          `json:"type"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

func (m FinalMask) MarshalJSON() ([]byte, error) {
	settings, err := json.Marshal(m.Fragment)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rawFinalMask{Type: m.Type, Settings: settings})
}

func (m *FinalMask) UnmarshalJSON(content []byte) error {
	var raw rawFinalMask
	if err := json.UnmarshalDisallowUnknownFields(content, &raw); err != nil {
		return err
	}
	m.Type = strings.ToLower(raw.Type)
	switch m.Type {
	case FinalMaskTypeFragment:
	case "":
		return E.New("finalmask: missing mask type")
	default:
		return E.New("finalmask: mask ", strconv.Quote(raw.Type), " is not supported")
	}
	if len(raw.Settings) == 0 {
		raw.Settings = []byte("{}")
	}
	// Как у Xray: лишние ключи в settings не роняют конфиг.
	if err := json.Unmarshal(raw.Settings, &m.Fragment); err != nil {
		return E.Cause(err, "finalmask fragment")
	}
	return m.Fragment.validate()
}

func (FinalMask) DescribeSchema(builder schema.Builder) (*schema.Node, error) {
	return builder.Define("FinalMask", func() (*schema.Node, error) {
		settings, err := builder.Describe(reflect.TypeFor[FragmentMaskOptions]())
		if err != nil {
			return nil, err
		}
		object := schema.StrictObject()
		object.Properties.Put("type", schema.StringEnum(FinalMaskTypeFragment))
		object.Properties.Put("settings", settings)
		object.Required = []string{"type"}
		return object, nil
	})
}

// FragmentMaskOptions — настройки маски fragment, семантика Xray
// (transport/internet/finalmask/fragment, infra/conf/transport_finalmask.go).
type FragmentMaskOptions struct {
	Packets  string             `json:"packets,omitempty"`
	Length   xbadoption.Range   `json:"length,omitempty"`
	Delay    xbadoption.Range   `json:"delay,omitempty"`
	Lengths  []xbadoption.Range `json:"lengths,omitempty"`
	Delays   []xbadoption.Range `json:"delays,omitempty"`
	MaxSplit xbadoption.Range   `json:"maxSplit,omitempty"`
}

// PacketsRange разбирает packets: "tlshello" → (0, 1), "" → (0, 0),
// "N" или "N-M" → (N, M), N ≥ 1.
func (o FragmentMaskOptions) PacketsRange() (from, to int64, err error) {
	switch strings.ToLower(o.Packets) {
	case "tlshello":
		return 0, 1, nil
	case "":
		return 0, 0, nil
	}
	var r xbadoption.Range
	if err = r.UnmarshalJSON([]byte(strconv.Quote(o.Packets))); err != nil {
		return 0, 0, E.Cause(err, "invalid packets")
	}
	if r.From <= 0 {
		return 0, 0, E.New("packets: from can't be 0")
	}
	return int64(r.From), int64(r.To), nil
}

func (o FragmentMaskOptions) LengthRanges() []xbadoption.Range {
	if len(o.Lengths) > 0 {
		return o.Lengths
	}
	return []xbadoption.Range{o.Length}
}

func (o FragmentMaskOptions) DelayRanges() []xbadoption.Range {
	if len(o.Delays) > 0 {
		return o.Delays
	}
	return []xbadoption.Range{o.Delay}
}

func (o FragmentMaskOptions) validate() error {
	if _, _, err := o.PacketsRange(); err != nil {
		return E.Cause(err, "finalmask fragment")
	}
	lengths := o.LengthRanges()
	for _, r := range lengths {
		if r.From < 0 {
			return E.New("finalmask fragment: negative length")
		}
	}
	if lengths[len(lengths)-1].From == 0 {
		return E.New("finalmask fragment: last length min can't be 0")
	}
	for _, r := range o.DelayRanges() {
		if r.From < 0 {
			return E.New("finalmask fragment: negative delay")
		}
	}
	if o.MaxSplit.From < 0 {
		return E.New("finalmask fragment: negative maxSplit")
	}
	return nil
}
