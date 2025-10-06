package nfc

import "github.com/clausecker/freefare"

// Tag represents an NFC tag at the hardware protocol level.
//
// Tag provides a unified interface for reading and writing NDEF data
// regardless of the underlying tag technology (MIFARE Classic, ISO14443-4, etc.).
//
// For most use cases, prefer using Card which provides a higher-level,
// io.Reader/Writer compatible API.
//
// Example:
//
//	tags, _ := manager.GetTags(device)
//	for _, tag := range tags {
//	    data, _ := tag.ReadData()
//	}
type Tag interface {
	UID() string
	Type() string
	NumericType() int
	ReadData() ([]byte, error)
	WriteData(data []byte) error
	Transceive(data []byte) ([]byte, error)
	Connect() error
	Disconnect() error
	IsWritable() (bool, error)
	CanMakeReadOnly() (bool, error)
	MakeReadOnly() error
}

// ClassicTag extends Tag with MIFARE Classic specific operations.
//
// ClassicTag provides low-level sector and block access for MIFARE Classic
// tags (1K and 4K variants).
//
// Example:
//
//	if classic, ok := tag.(ClassicTag); ok {
//	    key := [6]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
//	    data, _ := classic.Read(1, 0, key[:], 0x60)
//	}
type ClassicTag interface {
	Tag
	FreefareTagProvider
	Read(sector, block uint8, key []byte, keyType int) ([]byte, error)
	Write(sector, block uint8, data []byte, key []byte, keyType int) error
}

// ISO14443Tag extends Tag with ISO14443-4 (Type 4) specific operations.
//
// ISO14443Tag represents NFC Forum Type 4 tags which use ISO7816-4 APDUs
// for communication. These tags are commonly used in contactless payment
// and high-security applications.
type ISO14443Tag interface {
	Tag
	// Additional ISO14443-4 specific methods can be added here
}

// DESFireTag extends Tag with MIFARE DESFire specific operations.
//
// DESFireTag provides access to DESFire-specific features like applications,
// files, and authentication with various key types (DES, 3DES, AES).
//
// Example:
//
//	if desfire, ok := tag.(DESFireTag); ok {
//	    version, _ := desfire.Version()
//	    apps, _ := desfire.ApplicationIds()
//	}
type DESFireTag interface {
	Tag
	FreefareTagProvider
	Version() ([]byte, error)
	ApplicationIds() ([][]byte, error)
	SelectApplication(aid []byte) error
	Authenticate(keyNo byte, key []byte) error
	FileIds() ([]byte, error)
	ReadFile(fileNo byte, offset int64, length int) ([]byte, error)
	WriteFile(fileNo byte, offset int64, data []byte) error
}

// UltralightTag extends Tag with MIFARE Ultralight specific operations.
//
// UltralightTag provides page-based read/write operations for Ultralight cards.
//
// Example:
//
//	if ultralight, ok := tag.(UltralightTag); ok {
//	    data, _ := ultralight.ReadPage(4)
//	    ultralight.WritePage(5, [4]byte{0x01, 0x02, 0x03, 0x04})
//	}
type UltralightTag interface {
	Tag
	FreefareTagProvider
	ReadPage(page byte) ([4]byte, error)
	WritePage(page byte, data [4]byte) error
}

// FreefareTagProvider provides access to the underlying freefare.Tag object.
// This is used for advanced operations that require direct access to the
// freefare library.
type FreefareTagProvider interface {
	GetFreefareTag() freefare.Tag
}
