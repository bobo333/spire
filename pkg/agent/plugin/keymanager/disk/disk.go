package disk

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"sync"

	"github.com/hashicorp/hcl"
	"github.com/spiffe/spire/pkg/common/diskutil"
	"github.com/spiffe/spire/proto/agent/keymanager"

	spi "github.com/spiffe/spire/proto/common/plugin"
)

const keyFileName = "svid.key"

type pluginConfig struct {
	Directory string `hcl:"directory" json:"directory"`
}

type diskPlugin struct {
	mtx *sync.RWMutex
	dir string
}

func (d *diskPlugin) GenerateKeyPair(context.Context, *keymanager.GenerateKeyPairRequest) (*keymanager.GenerateKeyPairResponse, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	privData, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	pubData, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}

	resp := &keymanager.GenerateKeyPairResponse{PublicKey: pubData, PrivateKey: privData}
	return resp, nil
}

func (d *diskPlugin) StorePrivateKey(ctx context.Context, req *keymanager.StorePrivateKeyRequest) (*keymanager.StorePrivateKeyResponse, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	if d.dir == "" {
		return nil, errors.New("path not configured")
	}
	keyPath := path.Join(d.dir, keyFileName)

	if err := diskutil.AtomicWriteFile(keyPath, req.PrivateKey, 0600); err != nil {
		return nil, err
	}

	return &keymanager.StorePrivateKeyResponse{}, nil
}

func (d *diskPlugin) FetchPrivateKey(context.Context, *keymanager.FetchPrivateKeyRequest) (*keymanager.FetchPrivateKeyResponse, error) {
	// Start with empty response
	resp := &keymanager.FetchPrivateKeyResponse{PrivateKey: []byte{}}

	d.mtx.RLock()
	p := path.Join(d.dir, keyFileName)
	d.mtx.RUnlock()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return resp, nil
	}

	data, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	// Check key integrity first
	key, err := x509.ParseECPrivateKey(data)
	if err != nil {
		return nil, err
	}

	resp.PrivateKey, _ = x509.MarshalECPrivateKey(key)
	return resp, nil
}

func (d *diskPlugin) Configure(ctx context.Context, req *spi.ConfigureRequest) (*spi.ConfigureResponse, error) {
	config := &pluginConfig{}
	hclTree, err := hcl.Parse(req.Configuration)
	if err != nil {
		return nil, err
	}
	err = hcl.DecodeObject(&config, hclTree)
	if err != nil {
		return nil, err
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.dir = config.Directory
	return &spi.ConfigureResponse{}, nil
}

func (d *diskPlugin) GetPluginInfo(context.Context, *spi.GetPluginInfoRequest) (*spi.GetPluginInfoResponse, error) {
	return &spi.GetPluginInfoResponse{}, nil
}

func New() *diskPlugin {
	return &diskPlugin{
		mtx: new(sync.RWMutex),
	}
}
