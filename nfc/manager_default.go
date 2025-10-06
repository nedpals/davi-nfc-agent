package nfc

import (
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/clausecker/freefare"
	"github.com/clausecker/nfc/v2"
)

// defaultManager implements Manager using libnfc and freefare libraries.
type defaultManager struct{}

func (m *defaultManager) OpenDevice(deviceStr string) (Device, error) {
	dev, err := nfc.Open(deviceStr)
	if err != nil {
		return nil, err
	}
	return NewDevice(dev), nil
}

func (m *defaultManager) ListDevices() ([]string, error) {
	var devices []string
	var err error
	for i := 0; i < DeviceEnumRetries; i++ {
		devices, err = nfc.ListDevices()
		if err == nil {
			return devices, nil
		}
		time.Sleep(time.Millisecond * 100)
	}
	return nil, fmt.Errorf("failed to list NFC devices after %d retries: %w", DeviceEnumRetries, err)
}

// GetTags polls for tags on the given device.
// It first uses freefare.GetTags to find known Freefare-supported tags.
// Then, it polls for all ISO14443A targets and identifies Type 4A tags
// not already covered by Freefare.
func (m *defaultManager) GetTags(dev Device) ([]Tag, error) {
	libnfcDev, ok := dev.(*libnfcDevice)
	if !ok {
		return nil, fmt.Errorf("GetTags expects a libnfcDevice but got %T", dev)
	}

	var allFoundTags []Tag
	processedUIDs := make(map[string]bool)

	// 1. Get Freefare tags (MIFARE Classic, DESFire, etc.)
	ffTags, err := freefare.GetTags(libnfcDev.device)
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
				allFoundTags = append(allFoundTags, newClassicAdapter(t))
				processedUIDs[uid] = true
			case freefare.DESFireTag:
				allFoundTags = append(allFoundTags, newDESFireAdapter(t))
				processedUIDs[uid] = true
			case freefare.UltralightTag:
				allFoundTags = append(allFoundTags, newUltralightAdapter(t))
				processedUIDs[uid] = true
			default:
				log.Printf("Found other Freefare tag: UID %s, Type %T", uid, t)
				processedUIDs[uid] = true
			}
		}
	}

	// 2. Poll for ISO14443A tags (including Type 4)
	modulation := nfc.Modulation{Type: nfc.ISO14443a, BaudRate: nfc.Nbr106}
	nfcTargets, listErr := libnfcDev.device.InitiatorListPassiveTargets(modulation)
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

			// Check SAK for ISO14443-4 compliance (Type 4A: bit 5 = 0x20)
			if (isoATarget.Sak & 0x20) != 0 {
				log.Printf("Found ISO14443-4A tag: UID %s, SAK %02X", currentUID, isoATarget.Sak)
				allFoundTags = append(allFoundTags, newISO14443Adapter(target, libnfcDev))
				processedUIDs[currentUID] = true
			}
		}
	}

	return allFoundTags, nil
}
