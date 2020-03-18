package encoding

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unsafe"
)

var (
	errFieldsNum              = errors.New("incorrect number of fields in metric")
	errTimestampFormatInvalid = errors.New("timestamp is not unix ts format")
	errValueInvalid           = errors.New("value is not int or float")
	errFmtNullInKey           = "null char at position %d"
	errFmtNotAscii            = "non-ascii char at position %d"
)

const PlainFormat FormatName = "plain"

type PlainAdapter struct {
	Validate         bool
	UnsafeProcessing bool
}

func NewPlain(validate bool, unsafe bool) PlainAdapter {
	return PlainAdapter{Validate: validate, UnsafeProcessing: unsafe}
}

func (p PlainAdapter) validateKeyS(key string) error {
	if p.Validate {
		for i := 0; i < len(key); i++ {
			if key[i] == 0 {
				return fmt.Errorf(errFmtNullInKey, i)
			}
			if key[i] > unicode.MaxASCII {
				return fmt.Errorf(errFmtNotAscii, i)
			}
		}
	}
	return nil
}

func (p PlainAdapter) validateKey(key []byte) error {
	if p.Validate {
		for i := 0; i < len(key); i++ {
			if key[i] == 0 {
				return fmt.Errorf(errFmtNullInKey, i)
			}
			if key[i] > unicode.MaxASCII {
				return fmt.Errorf(errFmtNotAscii, i)
			}
		}
	}
	return nil
}

func (p PlainAdapter) KindS() string {
	return string(PlainFormat)
}

func (p PlainAdapter) Kind() FormatName {
	return PlainFormat
}

func (p PlainAdapter) Dump(dp Datapoint) []byte {
	return []byte(dp.String())
}

// This is doable since a byte slice can be converted to a string with no allocation provided we are a bit yolo
func fastString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func (p PlainAdapter) Load(msgbuf []byte) (Datapoint, error) {
	if p.UnsafeProcessing {
		return p.loadFaster(msgbuf)
	}
	return p.load(msgbuf)
}

func (p PlainAdapter) load(msgbuf []byte) (Datapoint, error) {
	d := Datapoint{}

	msg := string(msgbuf)
	start := 0
	for msg[start] == ' ' {
		start++
	}
	if msg[start] == '.' {
		start++
	}
	firstSpace := strings.IndexByte(msg[start:], ' ')
	if firstSpace == -1 {
		return d, errFieldsNum
	}
	firstSpace += start
	if err := p.validateKeyS(msg[start:firstSpace]); err != nil {
		return d, err
	}
	d.Name = msg[start:firstSpace]
	for msg[firstSpace] == ' ' {
		firstSpace++
	}
	nextSpace := strings.IndexByte(msg[firstSpace:], ' ')
	if nextSpace == -1 {
		return d, errFieldsNum
	}
	nextSpace += firstSpace
	v, err := strconv.ParseFloat(msg[firstSpace:nextSpace], 64)
	if err != nil {
		return d, err
	}
	d.Value = v
	for msg[nextSpace] == ' ' {
		nextSpace++
	}

	ts, err := strconv.ParseUint(msg[nextSpace:], 10, 32)
	if err != nil {
		return d, err
	}
	d.Timestamp = ts
	return d, nil
}
func (p PlainAdapter) loadFaster(msgbuf []byte) (Datapoint, error) {
	d := Datapoint{}

	msg := fastString(msgbuf)
	start := 0
	for msg[start] == ' ' {
		start++
	}
	if msg[start] == '.' {
		start++
	}
	firstSpace := strings.IndexByte(msg[start:], ' ')
	if firstSpace == -1 {
		return d, errFieldsNum
	}
	firstSpace += start
	if err := p.validateKeyS(msg[start:firstSpace]); err != nil {
		return d, err
	}
	d.Name = msg[start:firstSpace]
	for msg[firstSpace] == ' ' {
		firstSpace++
	}
	nextSpace := strings.IndexByte(msg[firstSpace:], ' ')
	if nextSpace == -1 {
		return d, errFieldsNum
	}
	nextSpace += firstSpace
	v, err := strconv.ParseFloat(msg[firstSpace:nextSpace], 64)
	if err != nil {
		return d, err
	}
	d.Value = v
	for msg[nextSpace] == ' ' {
		nextSpace++
	}

	ts, err := strconv.ParseUint(msg[nextSpace:], 10, 32)
	if err != nil {
		return d, err
	}
	d.Timestamp = ts
	return d, nil
}
