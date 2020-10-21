package ldrlib

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"log"
)

const (
	//RSAKeySize is the byte size of an RSA Key (ASN.1 DER)
	RSAKeySize = 459

	//RSAEncryptionSize is the byte size of the encryption output
	RSAEncryptionSize = 256

	//AESKeySize is the size for the AES keys (bytes) 126 bit keys
	AESKeySize = 16

	//AESBaseIVSize is the size for the AES IV (bytes)
	AESBaseIVSize = 12

	//AESStartSeqSize is the size for the AES StartSeq (bytes)
	AESStartSeqSize = 12

	//SignatureSize for an lnd signature (bytes)
	SignatureSize = 65
)

// GenerateRSAKeyPair generates a new key pair
func generateRSAKeyPair() ([]byte, []byte) {
	privkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	return privateRSAKeyToBytes(privkey), publicRSAKeyToBytes(&privkey.PublicKey)
}

//EncryptRSA encrypts the given message with RSA-OAEP.
func encryptRSA(pubkey []byte, message []byte) []byte {

	// crypto/rand.Reader is a good source of entropy for randomizing the
	// encryption function.
	rng := rand.Reader
	rsaPubKey := bytesToPublicRSAKey(pubkey)

	cipherText, err := rsa.EncryptOAEP(sha256.New(), rng, rsaPubKey, message, nil)
	if err != nil {
		log.Fatal(err)
	}

	return cipherText
}

//DecryptRSA decrypts the given message with RSA-OAEP.
func decryptRSA(privkey []byte, message []byte) []byte {

	rsaPrivKey := bytesToPrivateRSAKey(privkey)

	plainText, err := rsa.DecryptOAEP(sha256.New(), nil, rsaPrivKey, message, nil)
	if err != nil {
		log.Fatalln(err)
	}

	return plainText
}

// rsaPublicKeyToBytes transforms the public key to PKIX, ASN.1 DER form bytes
func publicRSAKeyToBytes(pub *rsa.PublicKey) []byte {
	pubASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		log.Fatal(err)
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubASN1,
	})

	return pubBytes
}

// privateRSAKeyToBytes transforms the RSA private key to PKCS#1, ASN.1 DER form
func privateRSAKeyToBytes(priv *rsa.PrivateKey) []byte {
	privBytes := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		},
	)

	return privBytes
}

// BytesToPrivateKey bytes to private key
func bytesToPrivateRSAKey(priv []byte) *rsa.PrivateKey {
	block, _ := pem.Decode(priv)
	enc := x509.IsEncryptedPEMBlock(block)
	b := block.Bytes
	var err error
	if enc {
		log.Println("is encrypted pem block")
		b, err = x509.DecryptPEMBlock(block, nil)
		if err != nil {
			log.Fatal(err)
		}
	}
	key, err := x509.ParsePKCS1PrivateKey(b)
	if err != nil {
		log.Fatal(err)
	}
	return key
}

// BytesToPublicKey bytes to public key
func bytesToPublicRSAKey(pub []byte) *rsa.PublicKey {
	block, _ := pem.Decode(pub)
	enc := x509.IsEncryptedPEMBlock(block)
	b := block.Bytes
	var err error
	if enc {
		log.Println("is encrypted pem block")
		b, err = x509.DecryptPEMBlock(block, nil)
		if err != nil {
			log.Fatal(err)
		}
	}
	ifc, err := x509.ParsePKIXPublicKey(b)
	if err != nil {
		log.Fatal(err)
	}
	key, ok := ifc.(*rsa.PublicKey)
	if !ok {
		log.Fatal("Not Ok")
	}
	return key
}

//CreateAESKey returns a valid aes key
func createAESKey() ([]byte, error) {
	return generateNRandomBytes(AESKeySize)
}

//GenerateNRandomBytes generates N random bytes and writes them into a byte slice
func generateNRandomBytes(n int) ([]byte, error) {

	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return nil, err
	}

	return b, nil
}

//EncryptAES encrypts the message using AES and GCM as the mode of operation for symmetric key cryptographic block ciphers
func encryptAES(key []byte, nonce []byte, message []byte) []byte {

	cphr, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}
	gcm, err := cipher.NewGCM(cphr)
	if err != nil {
		log.Fatal(err)
	}

	return gcm.Seal(nil, nonce, message, nil)
}

//DecryptAES encrypts the message using AES
func decryptAES(key []byte, nonce []byte, message []byte) []byte {
	cphr, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}
	gcm, err := cipher.NewGCM(cphr)
	if err != nil {
		log.Fatal(err)
	}

	plaintext, err := gcm.Open(nil, nonce, message, nil)
	if err != nil {
		log.Fatal(err)
	}

	return plaintext
}

func incrementSeqNumber(conn *connInfo) {

	index := len(conn.seqNumber) - 1

	for index >= 0 {
		if conn.seqNumber[index] < 255 {
			conn.seqNumber[index]++
			break
		} else {
			conn.seqNumber[index] = 0
			index--
		}
	}

	log.Println("Incremented sequence number to", conn.seqNumber)
}

func getNonce(baseIV []byte, seq []byte) []byte {

	nonce := make([]byte, len(baseIV))

	for i := 0; i < len(nonce); i++ {
		nonce[i] = baseIV[i] ^ seq[i]
	}

	return nonce
}
