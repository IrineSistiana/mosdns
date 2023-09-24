package edns0ede

import (
	"strings"

	"github.com/miekg/dns"
	"go.uber.org/zap/zapcore"
)

type EdeError dns.EDNS0_EDE

func (e *EdeError) Error() string {
	return (*dns.EDNS0_EDE)(e).String()
}

// MarshalLogObject implements zapcore.ObjectMarshaler.
func (e *EdeError) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddUint16("info_code", e.InfoCode)
	encoder.AddString("extra_text", e.ExtraText)
	return nil
}

type EdeErrors []*dns.EDNS0_EDE

func (e *EdeErrors) Error() string {
	sb := new(strings.Builder)
	if len(*e) == 0 {
		sb.WriteString("nil")
	} else {
		sb.WriteString((*e)[1].String())
	}
	for _, ede := range (*e)[1:] {
		sb.WriteString(", ")
		sb.WriteString(ede.String())
	}
	return sb.String()
}

func (e *EdeErrors) MarshalLogArray(ae zapcore.ArrayEncoder) error {
	for _, ede := range *e {
		ae.AppendObject((*EdeError)(ede))
	}
	return nil
}
