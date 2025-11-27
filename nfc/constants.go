package nfc

// Manager type constants for identifying different manager implementations
const (
	ManagerTypeHardware   = "hardware"
	ManagerTypeSmartphone = "smartphone"
)

// Card type constants for card type identification and filtering
const (
	CardTypeMifareClassic1K  = "MIFARE Classic 1K"
	CardTypeMifareClassic4K  = "MIFARE Classic 4K"
	CardTypeMifareUltralight = "MIFARE Ultralight"
	CardTypeNtag215          = "NTAG215"
	CardTypeDesfire          = "DESFire"
	CardTypeType4            = "Type4"
)

// GetAllCardTypes returns all supported card type constants
func GetAllCardTypes() []string {
	return []string{
		CardTypeMifareClassic1K,
		CardTypeMifareClassic4K,
		CardTypeMifareUltralight,
		CardTypeNtag215,
		CardTypeDesfire,
		CardTypeType4,
	}
}
