package userlib

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"io"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

type UUID = uuid.UUID

// RSA key size (in bits)
const rsaKeySizeBits = 2048

// AES block size (in bytes)
const AESBlockSizeBytes = aes.BlockSize

// AES key size (in bytes)
const AESKeySizeBytes = 16

// Output size (in bytes) of Hash and MAC
const HashSizeBytes = sha512.Size

// Debug print true/false
var DebugPrint = false

// Whether to enable the symbolic debugger logging
var SymbolicDebug = true

// DebugMsg. Helper function: Does formatted printing to stderr if
// the DebugPrint global is set.  All our testing ignores stderr,
// so feel free to use this for any sort of testing you want.

func SetDebugStatus(status bool) {
	DebugPrint = status
}

func DebugHeader(format string, args ...interface{}) {
	DebugMsg(strings.Repeat("-", 50))
	DebugMsg(format, args)
	DebugMsg(strings.Repeat("-", 50))
}

func DebugMsg(format string, args ...interface{}) {
	// if DebugPrint {
	msg := fmt.Sprintf("%v ", time.Now().Format("15:04:05.00000"))
	log.Printf(msg+strings.Trim(format, "\r\n ")+"\n", args...)
	// }
}

func stripQuotes(data []byte) (cleaned []byte) {
	return data
	// strData := string(data)
	// if len(strData) < 2 {
	// 	return data
	// } else if strData[0] == byte('"') && strData[len(strData)-1] == byte('"') {
	// 	return []byte(strData[1 : len(strData)-1])
	// } else {
	// 	return data
	// }
}

const TRUNCATE_LENGTH = 12

// Takes in byte slice data (with no text interpretation) and cuts it short
func truncateBytes(data []byte) (truncated string) {
	//data = stripQuotes(data)
	dataStr := fmt.Sprintf("%x", data)

	if len(dataStr) < TRUNCATE_LENGTH+3 {
		return fmt.Sprintf("%s", dataStr)
	} else {
		return fmt.Sprintf("%s...", dataStr[:TRUNCATE_LENGTH])
	}
}

// Takes in string data (with text interpretation) and cuts it short
func truncateStr(data string) (truncated string) {
	if len(data) < TRUNCATE_LENGTH+3 {
		return fmt.Sprintf("%s", data)
	} else {
		return fmt.Sprintf("%s...", data[:TRUNCATE_LENGTH])
	}
}

// Converts byte slice data to valid symbolic map keys.
func keyFromBytes(data []byte) (truncated string) {
	return fmt.Sprintf("%x", sha512.Sum512(data))
}

var Truncate = truncateBytes

// RandomBytes. Helper function: Returns a byte slice of the specified
// size filled with random data
func randomBytes(size int) (data []byte) {
	data = make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, data); err != nil {
		panic(err)
	}
	// t, _ := json.Marshal(data)
	record(data, "Rand(%s)", truncateBytes(data))
	// record(t, "Rand(%s)", truncate(data))
	return
}

// Can replace this function for development/testing
var RandomBytes = randomBytes

type PublicKeyType struct {
	KeyType string
	PubKey  rsa.PublicKey
}

type PrivateKeyType struct {
	KeyType string
	PrivKey rsa.PrivateKey
}

// Bandwidth tracker (for measuring efficient append)
var datastoreBandwidth = 0

// Datastore and Keystore variables
var datastore map[UUID][]byte = make(map[UUID][]byte)
var keystore map[string]PublicKeyType = make(map[string]PublicKeyType)

// Symbols Table
// Keys should ALWAYS be directly what is returned from keyFromBytes(data)
// where data is the data being represented and has type []byte.
// Everything stored in this table should be represented (in reality) as []byte.
var symbols map[string]string = make(map[string]string)

func DebugPrintDatastore() {
	msg := "\n\nDATASTORE:\n\n"
	for key, element := range datastore {
		msg += fmt.Sprintf("\n\n--------------------\n%s\n--------------------\n%s\n", resolve(key[:]), resolve(element))
	}
	DebugMsg("%s\n", msg)
}

// func DebugPrintDatastore() {
// 	msg := "\n\nSYMBOLS:\n\n"
// 	for key, element := range symbols {
// 		msg += fmt.Sprintf("\n\n--------------------\n%s\n--------------------\n%s\n", key, element)
// 	}
// 	DebugMsg("%s\n", msg)
// }

func resolveString(data string) string {
	extracted := data
	if data[0] == byte('"') {
		extracted = data[1 : len(data)-1]
	}

	recovered, err := base64.StdEncoding.DecodeString(extracted)
	if err == nil {
		if result, ok := symbols[keyFromBytes([]byte(recovered))]; ok {
			return result
		}
	}

	return data
}

func marshal(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m1 := regexp.MustCompile(`".*?"`)
	replaced := m1.ReplaceAllStringFunc(string(data), resolveString)

	m2 := regexp.MustCompile(`{"KeyType":"PKE","PrivKey":{.*?}}}`)
	replaced = m2.ReplaceAllStringFunc(replaced, resolveString)

	m3 := regexp.MustCompile(`{"KeyType":"DS","PrivKey":{.*?}}}`)
	replaced = m3.ReplaceAllStringFunc(replaced, resolveString)

	record(data, "Marshal(%s)", replaced)
	return data, nil
}

func unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

var Unmarshal = unmarshal
var Marshal = marshal

/*
********************************************
**        Symbolic Debug Functions        **
********************************************
 */
func resolve(data []byte) string {
	if result, found := symbols[keyFromBytes(data)]; found {
		return result
	}
	//If not found, assume it's a literal of some sort with text interpretation
	return truncateStr(string(data))
}

func record(key []byte, template string, values ...interface{}) {
	s := fmt.Sprintf(template, values...)
	symbols[keyFromBytes(key)] = s
	DebugMsg("%s => %s", truncateBytes(key), s)
}

func stringToByte(data string) (result []byte) {
	result, _ = json.Marshal(data)
	return
}

/*
********************************************
**           Datastore Functions          **
**       DatastoreSet, DatastoreGet,      **
**     DatastoreDelete, DatastoreClear    **
********************************************
 */

// Sets the value in the datastore
func datastoreSet(key UUID, value []byte) {
	// Update bandwidth tracker
	datastoreBandwidth += len(value)

	foo := make([]byte, len(value))
	copy(foo, value)

	datastore[key] = foo
}

var DatastoreSet = datastoreSet

// Returns the value if it exists
func datastoreGet(key UUID) (value []byte, ok bool) {
	value, ok = datastore[key]
	if ok && value != nil {
		// Update bandwidth tracker
		datastoreBandwidth += len(value)

		foo := make([]byte, len(value))
		copy(foo, value)
		return foo, ok
	}
	return
}

var DatastoreGet = datastoreGet

// Deletes a key
func datastoreDelete(key UUID) {
	delete(datastore, key)
}

var DatastoreDelete = datastoreDelete

// Use this in testing to reset the datastore to empty
func datastoreClear() {
	datastore = make(map[UUID][]byte)
}

var DatastoreClear = datastoreClear

func DatastoreResetBandwidth() {
	datastoreBandwidth = 0
}

// Get number of bytes uploaded/downloaded to/from Datastore.
func DatastoreGetBandwidth() int {
	return datastoreBandwidth
}

// Use this in testing to reset the keystore to empty
func keystoreClear() {
	keystore = make(map[string]PublicKeyType)
}

var KeystoreClear = keystoreClear

// Sets the value in the keystore
func keystoreSet(key string, value PublicKeyType) error {
	_, present := keystore[key]
	if present {
		return errors.New("entry in keystore has been taken")
	}

	keystore[key] = value
	return nil
}

var KeystoreSet = keystoreSet

// Returns the value if it exists
func keystoreGet(key string) (value PublicKeyType, ok bool) {
	value, ok = keystore[key]
	return
}

var KeystoreGet = keystoreGet

// Use this in testing to get the underlying map if you want
// to play with the datastore.
func DatastoreGetMap() map[UUID][]byte {
	return datastore
}

// Use this in testing to get the underlying map if you want
// to play with the keystore.
func KeystoreGetMap() map[string]PublicKeyType {
	return keystore
}

/*
********************************************
**               KDF                      **
**            Argon2Key                   **
********************************************
 */

// Argon2:  Automatically chooses a decent combination of iterations and memory
// Use this to generate a key from a password
func argon2Key(password []byte, salt []byte, keyLen uint32) []byte {
	result := argon2.IDKey(password, salt, 1, 64*1024, 4, keyLen)

	// Symbolic logging
	record(result, "Argon2Key(password=%s, salt=%s, keyLen=%d)", string(password), truncateStr(string(salt)), keyLen)

	return result
}

var Argon2Key = argon2Key

/*
********************************************
**               Hash                     **
**              SHA512                    **
********************************************
 */

// SHA512: Returns the checksum of data.
func hash(data []byte) []byte {
	hashVal := sha512.Sum512(data)
	// debugValues[hashVal] = fmt.Sprintf("Hash(%s)", data)
	result := hashVal[:]
	DebugMsg("Hashing: %s", string(data))
	record(result, "Hash(data=%s)", resolve(data))
	// record(result[:16], "Hash(data=%s)[:16]", resolve(data))

	return result // Converting from [64]byte array to []byte slice
}

// Hash returns a byte slice containing the SHA512 hash of the given byte slice.
var Hash = hash

func uuidNew() UUID {
	//use our randomness to allow seeding
	bytes := randomBytes(16)
	result, _ := uuid.FromBytes(bytes)
	record(bytes, "UUID(rand=%s)", truncateBytes(bytes))
	return result
}

var UUIDNew = uuidNew

func uuidFromBytes(b []byte) (result UUID, err error) {
	if len(b) < 16 {
		panic("UUIDFromBytes expects an input greater than or equal to 16 characters.")
	}
	result, err = uuid.FromBytes(b[:16])
	if err != nil {
		return uuid.New(), err
	}
	record(result[:], "UUID(bytes=%s)", resolve(b))
	return result, err
}

var UUIDFromBytes = uuidFromBytes

/*
********************************************
**         Public Key Encryption          **
**       PKEKeyGen, PKEEnc, PKEDec        **
********************************************
 */

// Four structs to help you manage your different keys
// You should only have 1 of each struct
// keyType should be either:
//     "PKE": encryption
//     "DS": authentication and integrity

type PKEEncKey = PublicKeyType
type PKEDecKey = PrivateKeyType

type DSSignKey = PrivateKeyType
type DSVerifyKey = PublicKeyType

func recordKeys(publicKey rsa.PublicKey, privateKey rsa.PrivateKey,
	publicKeyStruct interface{}, privateKeyStruct interface{},
	publicKeyFormat string, privateKeyFormat string) {

	publicKeyId := truncateBytes(x509.MarshalPKCS1PublicKey(&publicKey))
	privateKeyId := truncateBytes(x509.MarshalPKCS1PrivateKey(&privateKey))

	record(x509.MarshalPKCS1PublicKey(&publicKey), publicKeyFormat, publicKeyId)
	record(x509.MarshalPKCS1PrivateKey(&privateKey), privateKeyFormat, privateKeyId)

	// This is for the case where the key is used inside of a struct and we have to regex on the struct's marshalled type
	pub, err := json.Marshal(publicKeyStruct)
	if err != nil {
		panic("uhoh")
	}
	record(pub, publicKeyFormat, publicKeyId)

	priv, err := json.Marshal(privateKeyStruct)
	if err != nil {
		panic("uhoh")
	}
	record(priv, privateKeyFormat, privateKeyId)
}

// Generates a key pair for public-key encryption via RSA
func pkeKeyGen() (PKEEncKey, PKEDecKey, error) {
	RSAPrivKey, err := rsa.GenerateKey(rand.Reader, rsaKeySizeBits)
	RSAPubKey := RSAPrivKey.PublicKey

	var PKEEncKeyRes PKEEncKey
	PKEEncKeyRes.KeyType = "PKE"
	PKEEncKeyRes.PubKey = RSAPubKey

	var PKEDecKeyRes PKEDecKey
	PKEDecKeyRes.KeyType = "PKE"
	PKEDecKeyRes.PrivKey = *RSAPrivKey

	recordKeys(RSAPubKey, *RSAPrivKey, PKEEncKeyRes, PKEDecKeyRes, "PKEEncKey(%s)", "PKEDecKey(%s)")

	return PKEEncKeyRes, PKEDecKeyRes, err
}

var PKEKeyGen = pkeKeyGen

// Encrypts a byte stream via RSA-OAEP with sha512 as hash
func pkeEnc(ek PKEEncKey, plaintext []byte) ([]byte, error) {
	RSAPubKey := &ek.PubKey

	if ek.KeyType != "PKE" {
		return nil, errors.New("Using a non-PKE key for PKE.")
	}

	ciphertext, err := rsa.EncryptOAEP(sha512.New(), rand.Reader, RSAPubKey, plaintext, nil)

	if err != nil {
		return nil, err
	}

	record(ciphertext, "PKEEnc(ek=%s, plaintext=%s)", resolve(x509.MarshalPKCS1PublicKey(&ek.PubKey)), resolve(plaintext))

	return ciphertext, nil
}

var PKEEnc = pkeEnc

// Decrypts a byte stream encrypted with RSA-OAEP/sha512
func pkeDec(dk PKEDecKey, ciphertext []byte) ([]byte, error) {
	RSAPrivKey := &dk.PrivKey

	if dk.KeyType != "PKE" {
		return nil, errors.New("Using a non-PKE key for PKE.")
	}

	decryption, err := rsa.DecryptOAEP(sha512.New(), rand.Reader, RSAPrivKey, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	record(decryption, "PKEDec(dk=%s, ciphertext=%s)", resolve(x509.MarshalPKCS1PrivateKey(&dk.PrivKey)), resolve(ciphertext))

	return decryption, nil
}

var PKEDec = pkeDec

/*
********************************************
**           Digital Signature            **
**       DSKeyGen, DSSign, DSVerify       **
********************************************
 */

// Generates a key pair for digital signature via RSA
func dsKeyGen() (DSSignKey, DSVerifyKey, error) {
	RSAPrivKey, err := rsa.GenerateKey(rand.Reader, rsaKeySizeBits)
	RSAPubKey := RSAPrivKey.PublicKey

	var DSSignKeyRes DSSignKey
	DSSignKeyRes.KeyType = "DS"
	DSSignKeyRes.PrivKey = *RSAPrivKey

	var DSVerifyKeyRes DSVerifyKey
	DSVerifyKeyRes.KeyType = "DS"
	DSVerifyKeyRes.PubKey = RSAPubKey

	recordKeys(RSAPubKey, *RSAPrivKey, DSSignKeyRes, DSVerifyKeyRes, "DSVerifyKey(%s)", "DSSignKey(%s)")

	return DSSignKeyRes, DSVerifyKeyRes, err
}

var DSKeyGen = dsKeyGen

// Signs a byte stream via SHA256 and PKCS1v15
func dsSign(sk DSSignKey, msg []byte) ([]byte, error) {
	RSAPrivKey := &sk.PrivKey

	if sk.KeyType != "DS" {
		return nil, errors.New("Using a non-DS key for DS.")
	}

	hashed := sha512.Sum512(msg)

	sig, err := rsa.SignPKCS1v15(rand.Reader, RSAPrivKey, crypto.SHA512, hashed[:])
	if err != nil {
		return nil, err
	}

	// val, err := json.Marshal(sig)
	// if err != nil {
	// 	panic("uhoh")
	// }
	record(sig, "DSSign(sk=%s, msg=%s)", resolve(x509.MarshalPKCS1PrivateKey(&sk.PrivKey)), resolve(msg))

	return sig, nil
}

var DSSign = dsSign

// Verifies a signature signed with SHA256 and PKCS1v15
func dsVerify(vk DSVerifyKey, msg []byte, sig []byte) error {
	RSAPubKey := &vk.PubKey

	if vk.KeyType != "DS" {
		return errors.New("Using a non-DS key for DS.")
	}

	hashed := sha512.Sum512(msg)

	err := rsa.VerifyPKCS1v15(RSAPubKey, crypto.SHA512, hashed[:], sig)

	if err != nil {
		return err
	} else {
		return nil
	}
}

var DSVerify = dsVerify

/*
********************************************
**                HMAC                    **
**         HMACEval, HMACEqual            **
********************************************
 */

// Evaluate the HMAC using sha512
func hmacEval(key []byte, msg []byte) ([]byte, error) {
	if len(key) != 16 { // && len(key) != 24 && len(key) != 32 {
		panic(errors.New("The input as key for HMAC should be a 16-byte key."))
	}

	mac := hmac.New(sha512.New, key)
	mac.Write(msg)
	res := mac.Sum(nil)
	// debugValues[res] = fmt.Sprintf("HMAC(key=%s, msg=%s)", debugLookup(key), debugLookup(msg))

	record(res, "HMAC(key=%s, msg=%s)", resolve(key), resolve(msg))
	//record(res[:16], "HMAC(key=%s, msg=%s)[:16]", resolve(key), msg)

	return res, nil
}

var HMACEval = hmacEval

// Equals comparison for hashes/MACs
// Does NOT leak timing.
func hmacEqual(a []byte, b []byte) bool {
	return hmac.Equal(a, b)
}

var HMACEqual = hmacEqual

/*
********************************************
**   Hash-Based Key Derivation Function   **
**                 HashKDF                **
********************************************
 */

// HashKDF (uses the same algorithm as hmacEval, wrapped to provide a useful
// error)
func hashKDF(key []byte, msg []byte) ([]byte, error) {
	if len(key) != 16 { // && len(key) != 24 && len(key) != 32 {
		panic(errors.New("The input as key for HashKDF should be a 16-byte key."))
	}

	mac := hmac.New(sha512.New, key)
	mac.Write(msg)
	res := mac.Sum(nil)

	record(res, "HashKDF(key=%s, msg=%s)", resolve(key), resolve(msg))
	//record(res[:16], "HashKDF(key=%s, msg=%s)[:16]", resolve(key), resolve(msg))

	return res, nil
}

var HashKDF = hashKDF

/*
********************************************
**        Symmetric Encryption            **
**           SymEnc, SymDec               **
********************************************
 */

// Encrypts a byte slice with AES-CFB
// Length of iv should be == AESBlockSizeBytes
func symEnc(key []byte, iv []byte, plaintext []byte) []byte {
	if len(iv) != AESBlockSizeBytes {
		panic("IV length not equal to AESBlockSizeBytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// The IV needs to be unique, but not secret. Therefore it's common to
	// include it at the beginning of the ciphertext.
	ciphertext := make([]byte, AESBlockSizeBytes+len(plaintext))

	mode := cipher.NewCFBEncrypter(block, iv)
	mode.XORKeyStream(ciphertext[AESBlockSizeBytes:], plaintext)
	copy(ciphertext[:AESBlockSizeBytes], iv)

	record(ciphertext, "SymEnc(key=%s, iv=%s, plaintext=%s)", resolve(key), resolve(iv), resolve(plaintext))

	return ciphertext
}

var SymEnc = symEnc

// Decrypts a ciphertext encrypted with AES-CFB
func symDec(key []byte, ciphertext []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	if len(ciphertext) < aes.BlockSize {
		panic("ciphertext too short")
	}

	iv := ciphertext[:AESBlockSizeBytes]
	ciphertext = ciphertext[AESBlockSizeBytes:]

	plaintext := make([]byte, len(ciphertext))

	mode := cipher.NewCFBDecrypter(block, iv)
	mode.XORKeyStream(plaintext, ciphertext)

	// Debug logging
	// Can cause collisions in the symbol table, but since plaintexts should never
	// appear in the DataStore (and therefore never get resolved), this shouldn't
	// cause any issues.

	/*
		SymEnc("Hello") => 1234
		SymEnc("Hello") => 5678

		[5678: SymEnc("Hello"), 1234: SymEnc("Hello")]

		SymDec(1234) => SymDec(key=k, ciphertext=1234)



	*/

	record(plaintext, "SymDec(key=%s, ciphertext=%s)", resolve(key), resolve(ciphertext))

	return plaintext
}

var SymDec = symDec
