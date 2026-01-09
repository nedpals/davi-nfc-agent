package nfc

import (
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/clausecker/freefare"
	"github.com/clausecker/nfc/v2"
)

// libnfcDevice implements Device using an actual nfc.Device from libnfc.
type libnfcDevice struct {
	device nfc.Device
}

// NewDevice creates a new Device from an nfc.Device.
func NewDevice(dev nfc.Device) Device {
	return &libnfcDevice{device: dev}
}

func (d *libnfcDevice) Close() error {
	return d.device.Close()
}

func (d *libnfcDevice) InitiatorInit() error {
	return d.device.InitiatorInit()
}

func (d *libnfcDevice) String() string {
	return d.device.String()
}

func (d *libnfcDevice) Connection() string {
	return d.device.Connection()
}

// Capabilities returns the capabilities of this libnfc device.
func (d *libnfcDevice) Capabilities() DeviceCapabilities {
	return DeviceCapabilities{
		CanTransceive:     true,
		CanPoll:           true,
		DeviceType:        "libnfc",
		SupportedTagTypes: []string{"MIFARE Classic", "DESFire", "Ultralight", "NTAG", "ISO14443-4"},
		SupportsEvents:    false,
	}
}

// Transceive implements the Device Transceive method for raw data exchange.
func (d *libnfcDevice) Transceive(txData []byte) ([]byte, error) {
	var rxData [262]byte // Max buffer size for NFC
	count, err := d.device.InitiatorTransceiveBytes(txData, rxData[:], 0)
	if err != nil {
		return nil, fmt.Errorf("libnfcDevice.Transceive: %w", err)
	}
	return rxData[:count], nil
}

// GetTags polls for tags on the device.
// It first uses freefare.GetTags to find known Freefare-supported tags.
// Then, it polls for all ISO14443A targets and identifies Type 4A tags
// not already covered by Freefare.
func (d *libnfcDevice) GetTags() ([]Tag, error) {
	var allFoundTags []Tag
	processedUIDs := make(map[string]bool)

	// 1. Get Freefare tags (MIFARE Classic, DESFire, etc.)
	ffTags, err := freefare.GetTags(d.device)
	if err != nil {
		log.Printf("Error getting tags from freefare.GetTags: %v", err)
	} else {
		for _, ffTag := range ffTags {
			uid := ffTag.UID()
			if processedUIDs[uid] {
				continue
			}

			switch t := ffTag.(type) {
			case freefare.ClassicTag:
				allFoundTags = append(allFoundTags, NewClassicTag(t))
				processedUIDs[uid] = true
			case freefare.DESFireTag:
				allFoundTags = append(allFoundTags, NewDESFireTag(t))
				processedUIDs[uid] = true
			case freefare.UltralightTag:
				allFoundTags = append(allFoundTags, NewUltralightTag(t))
				processedUIDs[uid] = true
			default:
				log.Printf("Found other Freefare tag: UID %s, Type %T", uid, t)
				processedUIDs[uid] = true
			}
		}
	}

	// 2. Poll for ISO14443A tags (including Type 4)
	modulation := nfc.Modulation{Type: nfc.ISO14443a, BaudRate: nfc.Nbr106}
	nfcTargets, listErr := d.device.InitiatorListPassiveTargets(modulation)
	if listErr != nil {
		if err != nil && len(allFoundTags) == 0 {
			return nil, fmt.Errorf("error from freefare (%v) AND passive targets (%w)", err, listErr)
		}
		log.Printf("Error listing passive targets: %v", listErr)
	} else {
		for _, target := range nfcTargets {
			isoATarget, isISOA := target.(*nfc.ISO14443aTarget)
			if !isISOA {
				continue
			}

			var currentUID string
			if isoATarget.UIDLen > 0 && int(isoATarget.UIDLen) <= len(isoATarget.UID) {
				currentUID = strings.ToUpper(hex.EncodeToString(isoATarget.UID[:isoATarget.UIDLen]))
			} else {
				continue
			}

			if processedUIDs[currentUID] {
				continue
			}

			// Check for NTAG21x (ATQA: 0x0044, SAK: 0x00, 7-byte UID)
			// NTAG213/215/216 all share these characteristics
			atqa := uint16(isoATarget.Atqa[0]) | uint16(isoATarget.Atqa[1])<<8
			if atqa == 0x0044 && isoATarget.Sak == 0x00 && isoATarget.UIDLen == 7 {
				log.Printf("Found NTAG21x tag: UID %s, ATQA %04X, SAK %02X", currentUID, atqa, isoATarget.Sak)
				// Create tag via freefare.NewTag and wrap as NtagTag
				ffTag, tagErr := freefare.NewTag(d.device, isoATarget)
				if tagErr == nil {
					if ultralightTag, ok := ffTag.(freefare.UltralightTag); ok {
						allFoundTags = append(allFoundTags, NewNtagTag(ultralightTag))
						processedUIDs[currentUID] = true
						continue
					}
				} else {
					log.Printf("Error creating NTAG tag: %v", tagErr)
				}
			}

			// Check SAK for ISO14443-4 compliance (Type 4A: bit 5 = 0x20)
			if (isoATarget.Sak & 0x20) != 0 {
				log.Printf("Found ISO14443-4A tag: UID %s, SAK %02X", currentUID, isoATarget.Sak)
				allFoundTags = append(allFoundTags, NewISO14443Tag(target, d))
				processedUIDs[currentUID] = true
			}
		}
	}

	return allFoundTags, nil
}
