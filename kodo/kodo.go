package kodo

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/qiniu/go-sdk/v7/auth/qbox"
	qiniuStorage "github.com/qiniu/go-sdk/v7/storage"
	"github.com/chree188/go-storage/storage"
)

type Config struct {
	AccessKey string `mapstructure:"access_key" json:"access_key" yaml:"access_key"`
	Bucket    string `mapstructure:"bucket" json:"bucket" yaml:"bucket"`
	Domain    string `mapstructure:"domain" json:"domain" yaml:"domain"`
	SecretKey string `mapstructure:"secret_key" json:"secret_key" yaml:"secret_key"`
	IsSsl     bool   `mapstructure:"is_ssl" json:"is_ssl" yaml:"is_ssl"`
	IsPrivate bool   `mapstructure:"is_private" json:"is_private" yaml:"is_private"`
}

type kodo struct {
	config        *Config
	putPolicy     *qiniuStorage.PutPolicy
	mac           *qbox.Mac
	formUploader  *qiniuStorage.FormUploader
	bucketManager *qiniuStorage.BucketManager
}

var (
	k    *kodo
	once *sync.Once
)

func Init(config Config) (storage.Storage, error) {
	once = &sync.Once{}
	once.Do(func() {
		k = &kodo{}
		k.config = &config

		k.putPolicy = &qiniuStorage.PutPolicy{
			Scope: config.Bucket,
		}
		k.mac = qbox.NewMac(config.AccessKey, config.SecretKey)

		cfg := qiniuStorage.Config{
			UseHTTPS:      config.IsSsl,
			UseCdnDomains: false,
		}

		k.formUploader = qiniuStorage.NewFormUploader(&cfg)
		k.bucketManager = qiniuStorage.NewBucketManager(k.mac, &cfg)

		storage.Register(storage.KoDo, k)
	})
	return k, nil
}

func (k *kodo) Put(key string, r io.Reader, dataLength int64, contentType string) error {
	key = storage.NormalizeKey(key)

	upToken := k.putPolicy.UploadToken(k.mac)
	ret := qiniuStorage.PutRet{}
	putExtra := qiniuStorage.PutExtra{MimeType: contentType}
	err := k.formUploader.Put(context.Background(), &ret, upToken, key, r, dataLength, &putExtra)
	if err != nil {
		return err
	}

	return nil
}

func (k *kodo) PutFile(key string, localFile string, contentType string) error {
	key = storage.NormalizeKey(key)

	upToken := k.putPolicy.UploadToken(k.mac)
	ret := qiniuStorage.PutRet{}
	putExtra := qiniuStorage.PutExtra{MimeType: contentType}
	err := k.formUploader.PutFile(context.Background(), &ret, upToken, key, localFile, &putExtra)
	if err != nil {
		return err
	}

	return nil
}

func (k *kodo) Get(key string) (io.ReadCloser, error) {
	key = storage.NormalizeKey(key)

	resp, err := http.Get(k.Url(key))
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (k *kodo) Rename(srcKey string, destKey string) error {
	srcKey = storage.NormalizeKey(srcKey)
	destKey = storage.NormalizeKey(destKey)

	err := k.bucketManager.Move(k.config.Bucket, srcKey, k.config.Bucket, destKey, true)
	if err != nil {
		return err
	}

	return nil
}

func (k *kodo) Copy(srcKey string, destKey string) error {
	srcKey = storage.NormalizeKey(srcKey)
	destKey = storage.NormalizeKey(destKey)

	err := k.bucketManager.Copy(k.config.Bucket, srcKey, k.config.Bucket, destKey, true)
	if err != nil {
		return err
	}

	return nil
}

func (k *kodo) Exists(key string) (bool, error) {
	key = storage.NormalizeKey(key)

	_, err := k.bucketManager.Stat(k.config.Bucket, key)
	if err != nil {
		if err.Error() == "no such file or directory" {
			err = nil
		}
		return false, err
	}

	return true, nil
}

func (k *kodo) Size(key string) (int64, error) {
	key = storage.NormalizeKey(key)

	fileInfo, err := k.bucketManager.Stat(k.config.Bucket, key)
	if err != nil {
		return 0, err
	}

	return fileInfo.Fsize, nil
}

func (k *kodo) Delete(key string) error {
	key = storage.NormalizeKey(key)

	err := k.bucketManager.Delete(k.config.Bucket, key)
	if err != nil {
		return err
	}

	return nil
}

func (k *kodo) Url(key string) string {
	var prefix string

	key = storage.NormalizeKey(key)

	if k.config.IsSsl {
		prefix = "https://"
	} else {
		prefix = "http://"
	}

	if k.config.IsPrivate {
		deadline := time.Now().Add(time.Second * 3600).Unix() // 1小时有效期
		return prefix + qiniuStorage.MakePrivateURL(k.mac, k.config.Domain, key, deadline)
	}

	return prefix + qiniuStorage.MakePublicURL(k.config.Domain, key)
}
