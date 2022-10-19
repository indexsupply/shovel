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
)

const (
	ef1  = "enr:-Ku4QHqVeJ8PPICcWk1vSn_XcSkjOkNiTg6Fmii5j6vUQgvzMc9L1goFnLKgXqBJspJjIsB91LTOleFmyWWrFVATGngBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpC1MD8qAAAAAP__________gmlkgnY0gmlwhAMRHkWJc2VjcDI1NmsxoQKLVXFOhp2uX6jeT0DvvDpPcU8FWMjQdR4wMuORMhpX24N1ZHCCIyg"
	ef2  = "enr:-Ku4QG-2_Md3sZIAUebGYT6g0SMskIml77l6yR-M_JXc-UdNHCmHQeOiMLbylPejyJsdAPsTHJyjJB2sYGDLe0dn8uYBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpC1MD8qAAAAAP__________gmlkgnY0gmlwhBLY-NyJc2VjcDI1NmsxoQORcM6e19T1T9gi7jxEZjk_sjVLGFscUNqAY9obgZaxbIN1ZHCCIyg"
	ef3  = "enr:-Ku4QPn5eVhcoF1opaFEvg1b6JNFD2rqVkHQ8HApOKK61OIcIXD127bKWgAtbwI7pnxx6cDyk_nI88TrZKQaGMZj0q0Bh2F0dG5ldHOIAAAAAAAAAACEZXRoMpC1MD8qAAAAAP__________gmlkgnY0gmlwhDayLMaJc2VjcDI1NmsxoQK2sBOLGcUb4AwuYzFuAVCaNHA-dy24UuEKkeFNgCVCsIN1ZHCCIyg"
	ef4  = "enr:-Ku4QEWzdnVtXc2Q0ZVigfCGggOVB2Vc1ZCPEc6j21NIFLODSJbvNaef1g4PxhPwl_3kax86YPheFUSLXPRs98vvYsoBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpC1MD8qAAAAAP__________gmlkgnY0gmlwhDZBrP2Jc2VjcDI1NmsxoQM6jr8Rb1ktLEsVcKAPa08wCsKUmvoQ8khiOl_SLozf9IN1ZHCCIyg"
	te1  = "enr:-KG4QOtcP9X1FbIMOe17QNMKqDxCpm14jcX5tiOE4_TyMrFqbmhPZHK_ZPG2Gxb1GE2xdtodOfx9-cgvNtxnRyHEmC0ghGV0aDKQ9aX9QgAAAAD__________4JpZIJ2NIJpcIQDE8KdiXNlY3AyNTZrMaEDhpehBDbZjM_L9ek699Y7vhUJ-eAdMyQW_Fil522Y0fODdGNwgiMog3VkcIIjKA"
	pry1 = "enr:-Ku4QImhMc1z8yCiNJ1TyUxdcfNucje3BGwEHzodEZUan8PherEo4sF7pPHPSIB1NNuSg5fZy7qFsjmUKs2ea1Whi0EBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpD1pf1CAAAAAP__________gmlkgnY0gmlwhBLf22SJc2VjcDI1NmsxoQOVphkDqal4QzPMksc5wnpuC3gvSC8AfbFOnZY_On34wIN1ZHCCIyg"
	lh1  = "enr:-IS4QLkKqDMy_ExrpOEWa59NiClemOnor-krjp4qoeZwIw2QduPC-q7Kz4u1IOWf3DDbdxqQIgC4fejavBOuUPy-HE4BgmlkgnY0gmlwhCLzAHqJc2VjcDI1NmsxoQLQSJfEAHZApkm5edTCZ_4qps_1k_ub2CxHFxi-gr2JMIN1ZHCCIyg"
)

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
	binary.BigEndian.PutUint64(buf[4:], n)
	return buf[:12]
}

func encodeUint16(n uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf[:], n)
	return buf[:2]
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

	iv := make([]byte, aes.BlockSize)
	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		return nil, nil, err
	}
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(header, header)
	return iv, header, nil
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
	//r, err := enr.UnmarshalFromText(lh1)
	//if err != nil {
	//	return "", err
	//}
	//addr := netip.AddrPortFrom(netip.AddrFrom4(r.Ip), r.UdpPort)
	//conn, err := net.Dial("udp", addr.String())
	conn, err := net.Dial("udp", "3.209.45.79:30303")
	if err != nil {
		return "", err
	}

	srcID, _ := hex.DecodeString("e5c64cb47159f91b8686c1c52620b313227c5cabdb57185192e6cbeff13c2a38")
	desID, _ := hex.DecodeString("f23ac6da7c02f84a425a47414be12dc2f62172cd16bd4c7e7efa02ebaa045605")
	msg := make([]byte, 20)

	rand.Read(msg)

	m, err := packet(srcID, 0, 1, desID[:16], msg)
	if err != nil {
		return "", err
	}

	fmt.Printf(">%x %d\n", m, len(m))
	_, err = conn.Write(m)
	if err != nil {
		return "", err
	}
	conn.Close()

	return "", nil
}
