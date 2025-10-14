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

// FreefareTagProvider provides access to the underlying freefare.Tag object.
// This is used for advanced operations that require direct access to the
// freefare library.
type FreefareTagProvider interface {
	GetFreefareTag() freefare.Tag
}
