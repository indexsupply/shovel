package enr

import (
	"strings"
	"errors"
	"encoding/base64"
	"fmt"

	"github.com/indexsupply/lib/rlp"
)

type ENR struct{
	ID string 
	Secp256k1 []byte

	Ip [4]byte
	TcpPort int
	UdpPort int

	Ip6 [6]byte
	Tcp6Port int
	Udp6Port int
}

const enrTextPrefix = "enr:"

func FromTextEncoding(str string) (*ENR, error) {
	if !strings.HasPrefix(str, enrTextPrefix) {
		return nil, errors.New("Invalid prefix for ENR text encoding.")
	}

	str = str[len(enrTextPrefix):]
	b, err := base64.RawURLEncoding.DecodeString(str)
	if err != nil {
		return nil, err
	}
	fmt.Println(len(b))

	item, err := rlp.Decode(b)
	if err != nil {
		return nil, err
	}

	fmt.Printf("%s", item)

	return &ENR{}, nil
}