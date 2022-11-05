// Small wrapper around sha3 package to
// canonicalize how data is to be hashed
package isxhash

import "golang.org/x/crypto/sha3"

func Keccak(d []byte) []byte {
	k := sha3.NewLegacyKeccak256()
	k.Write(d)
	return k.Sum(nil)
}
