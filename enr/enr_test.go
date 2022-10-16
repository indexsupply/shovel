package enr

import (
	"testing"
)

func TestFromTextEncoding(t *testing.T) {
	enrSample := "enr:-IS4QHCYrYZbAKWCBRlAy5zzaDZXJBGkcnh4MHcBFZntXNFrdvJjX04jRzjzCBOonrkTfj499SZuOh8R33Ls8RRcy5wBgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPKY0yuDUmstAHYpMa2_oxVtw0RW_QAdpzBQA8yWM0xOIN1ZHCCdl8"

	_, err := FromTextEncoding(enrSample)
	if err != nil {
		t.Fatal(err)
	}
}