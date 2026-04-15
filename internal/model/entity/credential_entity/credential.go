package credential_entity

import "errors"

// 凭证类型常量
const (
	TypePassword = "password"
	TypeSSHKey   = "ssh_key"
)

// SSH 密钥类型常量
const (
	KeyTypeRSA     = "rsa"
	KeyTypeED25519 = "ed25519"
	KeyTypeECDSA   = "ecdsa"
)

// Credential 统一凭证实体
type Credential struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Type        string `gorm:"column:type;type:varchar(50);not null" json:"type"` // "password" | "ssh_key"
	Username    string `gorm:"column:username;type:varchar(255)" json:"username,omitempty"`
	Password    string `gorm:"column:password;type:text" json:"-"`    // 加密后的密码（type=password）
	PrivateKey  string `gorm:"column:private_key;type:text" json:"-"` // 加密后的私钥（type=ssh_key）
	Passphrase  string `gorm:"column:passphrase;type:text" json:"-"`  // 加密后的私钥密码（type=ssh_key）
	PublicKey   string `gorm:"column:public_key;type:text" json:"publicKey,omitempty"`
	KeyType     string `gorm:"column:key_type;type:varchar(50)" json:"keyType,omitempty"` // rsa/ed25519/ecdsa
	KeySize     int    `gorm:"column:key_size" json:"keySize,omitempty"`
	Fingerprint string `gorm:"column:fingerprint;type:varchar(255)" json:"fingerprint,omitempty"`
	Comment     string `gorm:"column:comment;type:varchar(255)" json:"comment,omitempty"`
	Description string `gorm:"column:description;type:text" json:"description,omitempty"`
	Createtime  int64  `gorm:"column:createtime" json:"createtime"`
	Updatetime  int64  `gorm:"column:updatetime" json:"updatetime"`
}

// TableName GORM 表名
func (Credential) TableName() string {
	return "credentials"
}

// IsPassword 判断是否密码类型
func (c *Credential) IsPassword() bool {
	return c.Type == TypePassword
}

// IsSSHKey 判断是否 SSH 密钥类型
func (c *Credential) IsSSHKey() bool {
	return c.Type == TypeSSHKey
}

// Validate 校验
func (c *Credential) Validate() error {
	if c.Name == "" {
		return errors.New("凭证名称不能为空")
	}
	switch c.Type {
	case TypePassword:
		if c.Password == "" {
			return errors.New("密码不能为空")
		}
	case TypeSSHKey:
		if c.PrivateKey == "" {
			return errors.New("私钥不能为空")
		}
		if c.PublicKey == "" {
			return errors.New("公钥不能为空")
		}
		switch c.KeyType {
		case KeyTypeRSA, KeyTypeED25519, KeyTypeECDSA:
		default:
			return errors.New("不支持的密钥类型")
		}
	default:
		return errors.New("不支持的凭证类型")
	}
	return nil
}
