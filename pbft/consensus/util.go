package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"math/big"
	"crypto/ecdsa"
	"crypto/rand"
	// mrand "math/rand"
)

func Hash(content []byte) string {
	h := sha256.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func Digest(object interface{}) (string, error) {
	msg, err := json.Marshal(object)

	if err != nil {
		return "", err
	}

	return Hash(msg), nil
}
func Sign(privKey *ecdsa.PrivateKey, data []byte) (*big.Int, *big.Int, []byte, error) {
	r := big.NewInt(0)
	s := big.NewInt(0)
	signHash := sha256.Sum256(data)

	r, s, err := ecdsa.Sign(rand.Reader, privKey, signHash[:])

	if err != nil {
		return nil, nil, nil, err
	}

	signature := r.Bytes()
	signature = append(signature, s.Bytes()...)

	return r, s, signature, nil
}

func Verify(pubKey *ecdsa.PublicKey, r, s *big.Int, data []byte) bool {
	signHash := sha256.Sum256(data)
	return ecdsa.Verify(pubKey, signHash[:], r, s)
}

func NumOfPhase(s string) int64 {
	switch s{
	case "Prepare":
		return 0
	case "Vote":
		return 1
	case "Collate":
		return 2
	case "ViewChange":
		return 3
	case "Total":
		return 4
	}	
	return -1
}
