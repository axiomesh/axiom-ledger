package crypto

import (
	"encoding/json"
	"os"
	"reflect"

	"github.com/pkg/errors"
	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"

	"github.com/axiomesh/axiom-kit/hexutil"
)

const (
	KeyTypeEd25519  = "ed25519"
	KeyTypeBls12381 = "bls12381"
)

type Ed25519Keystore = Keystore[*Ed25519PrivateKey, *Ed25519PublicKey]

type Bls12381Keystore = Keystore[*Bls12381PrivateKey, *Bls12381PublicKey]

type KeystoreKey interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}

type PrivateKey interface {
	KeystoreKey
	Sign([]byte) ([]byte, error)
}

type KeystoreInfo struct {
	KeyType     string            `json:"key_type"`
	PrivateKey  map[string]any    `json:"private_key"`
	PublicKey   string            `json:"public_key"`
	Version     uint              `json:"version"`
	Description string            `json:"description"`
	Extra       map[string]string `json:"extra"`
}

type Keystore[PriKey KeystoreKey, PubKey KeystoreKey] struct {
	version             uint
	encryptedPrivateKey map[string]any

	Path        string
	KeyType     string
	Description string
	PrivateKey  PriKey
	PublicKey   PubKey
	Password    string
	Extra       map[string]string
}

func ReadKeystoreInfo(path string) (*KeystoreInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read keystore %s", path)
	}
	var info KeystoreInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal keystore %s", path)
	}
	return &info, nil
}

func WriteKeystoreInfo(path string, info *KeystoreInfo) error {
	raw, err := json.MarshalIndent(info, "", "\t")
	if err != nil {
		return errors.Wrapf(err, "failed to marshal keystore %s", path)
	}

	if err := os.WriteFile(path, raw, 0755); err != nil {
		return errors.Wrapf(err, "failed to write keystore %s", path)
	}
	return nil
}

func ReadKeystore[PriKey KeystoreKey, PubKey KeystoreKey](path string) (*Keystore[PriKey, PubKey], error) {
	info, err := ReadKeystoreInfo(path)
	if err != nil {
		return nil, err
	}

	publicKeyBytes := hexutil.Decode(info.PublicKey)
	publicKey := initPointer[PubKey]()
	if err := publicKey.Unmarshal(publicKeyBytes); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal public key in keystore %s", path)
	}

	return &Keystore[PriKey, PubKey]{
		version:             info.Version,
		encryptedPrivateKey: info.PrivateKey,
		Path:                path,
		KeyType:             info.KeyType,
		Description:         info.Description,
		PublicKey:           publicKey,
		Extra:               info.Extra,
	}, nil
}

func (ks *KeystoreInfo) UpdatePassword(oldPassword string, newPassword string) error {
	privateKeyBytes, err := ks.DecryptPrivateKey(oldPassword)
	if err != nil {
		return err
	}
	encoder := keystorev4.New()
	encryptedPrivateKey, err := encoder.Encrypt(privateKeyBytes, newPassword)
	if err != nil {
		return errors.Wrapf(err, "failed to encrypt private key")
	}
	ks.PrivateKey = encryptedPrivateKey
	return nil
}

func (ks *KeystoreInfo) DecryptPrivateKey(password string) ([]byte, error) {
	decoder := keystorev4.New()
	if ks.Version != decoder.Version() {
		return nil, errors.Errorf("invalid version %d, expected %d", ks.Version, decoder.Version())
	}

	privateKeyBytes, err := decoder.Decrypt(ks.PrivateKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "invalid password")
	}
	return privateKeyBytes, nil
}

func (ks *Keystore[PriKey, PubKey]) DecryptPrivateKey(password string) error {
	decoder := keystorev4.New()
	if ks.version != decoder.Version() {
		return errors.Errorf("keystore %s has invalid version %d, expected %d", ks.Path, ks.version, decoder.Version())
	}

	privateKeyBytes, err := decoder.Decrypt(ks.encryptedPrivateKey, password)
	if err != nil {
		return errors.Wrapf(err, "failed to decrypt private key in keystore %s", ks.Path)
	}
	privateKey := initPointer[PriKey]()
	if err := privateKey.Unmarshal(privateKeyBytes); err != nil {
		return errors.Wrapf(err, "failed to unmarshal private key in keystore %s", ks.Path)
	}

	ks.PrivateKey = privateKey
	ks.Password = password
	return nil
}

func (ks *Keystore[PriKey, PubKey]) Write() error {
	privateKeyBytes, err := ks.PrivateKey.Marshal()
	if err != nil {
		return errors.Wrapf(err, "failed to marshal private key in keystore %s", ks.Path)
	}
	publicKeyBytes, err := ks.PublicKey.Marshal()
	if err != nil {
		return errors.Wrapf(err, "failed to marshal public key in keystore %s", ks.Path)
	}

	encoder := keystorev4.New()
	encryptedPrivateKey, err := encoder.Encrypt(privateKeyBytes, ks.Password)
	if err != nil {
		return errors.Wrapf(err, "failed to encrypt private key in keystore %s", ks.Path)
	}
	return WriteKeystoreInfo(ks.Path, &KeystoreInfo{
		KeyType:     ks.KeyType,
		PrivateKey:  encryptedPrivateKey,
		PublicKey:   hexutil.Encode(publicKeyBytes),
		Version:     encoder.Version(),
		Description: ks.Description,
		Extra:       ks.Extra,
	})
}

func initPointer[P any]() P {
	var p P
	typ := reflect.TypeOf(p)
	if typ.Kind() != reflect.Ptr {
		panic("p must be a pointer")
	}
	ptr := reflect.New(typ.Elem()).Interface()
	return ptr.(P)
}

func IsZeroBytes(bytes []byte) bool {
	b := byte(0)
	for _, s := range bytes {
		b |= s
	}
	return b == 0
}
