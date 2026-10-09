package secretkey

// legacyKeyString 旧版固定加密密钥串：历史加密实现将其整串拷入 32 字节钥数组
// （超长部分截断）。仅用于解密旧格式密文（无 v2: 前缀的裸 base64），新加密
// 永不使用该钥
const legacyKeyString = "github.com/library-squirrel/wails-secure-key-32byte!"

// legacyKey 旧版固定钥：整串拷贝进 32 字节数组、超长截断，与历史实现的拷贝
// 方式逐字节一致
func legacyKey() [keySize]byte {
	var key [keySize]byte
	copy(key[:], []byte(legacyKeyString))
	return key
}
