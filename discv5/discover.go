package discv5

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/netip"

	"github.com/indexsupply/lib/enr"
)

const ef1 = "enr:-Ku4QHqVeJ8PPICcWk1vSn_XcSkjOkNiTg6Fmii5j6vUQgvzMc9L1goFnLKgXqBJspJjIsB91LTOleFmyWWrFVATGngBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpC1MD8qAAAAAP__________gmlkgnY0gmlwhAMRHkWJc2VjcDI1NmsxoQKLVXFOhp2uX6jeT0DvvDpPcU8FWMjQdR4wMuORMhpX24N1ZHCCIyg"

func Serve() error {
	conn, err := net.ListenPacket("udp", ":30303")
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		buf := make([]byte, 1024)
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			fmt.Printf("server read error: %s", err)
			continue
		}
		go serve(conn, addr, buf[:n])
	}
}

func serve(c net.PacketConn, addr net.Addr, buf []byte) {
	fmt.Printf("< %x\n", buf)
}

func nonce(n uint64) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint64(buf[:], n)
	return buf[:12]
}

func encodeUint16(n uint16) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf[:], n)
	return buf[:4]
}

func encodeUint64(n uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf[:], n)
	return buf[:8]
}

func header(authData []byte, flag byte, n uint64) []byte {
	var (
		protocolID = []byte("discv5")
		version    = []byte{0x00, 0x01}
	)

	var sh []byte
	sh = append(sh, protocolID...)
	sh = append(sh, version...)
	sh = append(sh, flag)
	sh = append(sh, nonce(n)...)
	sh = append(sh, encodeUint16(uint16(len(authData)))...)

	var h []byte
	h = append(h, sh...)
	h = append(h, authData...)
	return h
}

func maskHeader(key, header []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	ciphertext := make([]byte, aes.BlockSize+len(header))
	iv := ciphertext[:aes.BlockSize]
	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		return nil, nil, err
	}
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], header)
	return iv, ciphertext, nil
}

func packet(
	authData []byte,
	flag byte,
	n uint64,
	dest []byte,
	msg []byte,
) ([]byte, error) {
	h := header(authData, flag, n)

	iv, mh, err := maskHeader(dest, h)
	if err != nil {
		return nil, err
	}

	var p []byte
	p = append(p, iv...)
	p = append(p, mh...)
	p = append(p, msg...)
	return p, nil
}

func Random() (string, error) {
	r, err := enr.UnmarshalFromText(ef1)
	if err != nil {
		return "", err
	}
	addr := netip.AddrPortFrom(netip.AddrFrom4(r.Ip), r.UdpPort)
	conn, err := net.Dial("udp", addr.String())
	if err != nil {
		return "", err
	}

	var (
		srcID, _ = hex.DecodeString("aaaa8419e9f49d0083561b48287df592939a8d19947d8c0ef88f2a4856a69fbb")
		msg      = []byte{0x01, 0x02, 0x03, 0x04}
	)

	m, err := packet(srcID, 0, 1, r.Secp256k1[:16], msg)
	if err != nil {
		return "", err
	}

	fmt.Printf(">%x\n", m)
	_, err = conn.Write(m)
	if err != nil {
		return "", err
	}
	conn.Close()

	return "", nil
}
