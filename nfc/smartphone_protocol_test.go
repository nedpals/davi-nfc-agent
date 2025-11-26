package nfc

import (
	"testing"
	"time"
)

func TestParseSmartphoneUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "colon-separated uppercase",
			input:   "04:AB:CD:EF",
			want:    "04:AB:CD:EF",
			wantErr: false,
		},
		{
			name:    "colon-separated lowercase",
			input:   "04:ab:cd:ef",
			want:    "04:AB:CD:EF",
			wantErr: false,
		},
		{
			name:    "no separator",
			input:   "04ABCDEF",
			want:    "04:AB:CD:EF",
			wantErr: false,
		},
		{
			name:    "space separator",
			input:   "04 AB CD EF",
			want:    "04:AB:CD:EF",
			wantErr: false,
		},
		{
			name:    "dash separator",
			input:   "04-AB-CD-EF",
			want:    "04:AB:CD:EF",
			wantErr: false,
		},
		{
			name:    "7-byte UID",
			input:   "04:AB:CD:12:34:56:78",
			want:    "04:AB:CD:12:34:56:78",
			wantErr: false,
		},
		{
			name:    "empty UID",
			input:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid characters",
			input:   "04:AB:XY:EF",
			want:    "",
			wantErr: true,
		},
		{
			name:    "odd number of characters",
			input:   "04ABC",
			want:    "",
			wantErr: true,
		},
		{
			name:    "too short",
			input:   "A",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSmartphoneUID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSmartphoneUID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseSmartphoneUID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertNDEFRecordData(t *testing.T) {
	tests := []struct {
		name    string
		input   NDEFRecordData
		wantErr bool
	}{
		{
			name: "valid text record",
			input: NDEFRecordData{
				TNF:        0x01,
				Type:       []byte("T"),
				Payload:    []byte("test payload"),
				RecordType: "text",
				Content:    "test",
			},
			wantErr: false,
		},
		{
			name: "valid URI record",
			input: NDEFRecordData{
				TNF:        0x01,
				Type:       []byte("U"),
				Payload:    []byte{0x00, 'h', 't', 't', 'p', 's', ':', '/', '/', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'},
				RecordType: "uri",
				Content:    "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "invalid TNF",
			input: NDEFRecordData{
				TNF:     0x08,
				Type:    []byte("T"),
				Payload: []byte("test"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := ConvertNDEFRecordData(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertNDEFRecordData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && record == nil {
				t.Error("ConvertNDEFRecordData() returned nil record")
			}
		})
	}
}

func TestConvertNDEFMessageData(t *testing.T) {
	tests := []struct {
		name    string
		input   *NDEFMessageData
		wantErr bool
	}{
		{
			name: "valid message with text record",
			input: &NDEFMessageData{
				Records: []NDEFRecordData{
					{
						TNF:        0x01,
						Type:       []byte("T"),
						Payload:    MakeTextRecordPayload("Hello", "en"),
						RecordType: "text",
						Content:    "Hello",
						Language:   "en",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid message with multiple records",
			input: &NDEFMessageData{
				Records: []NDEFRecordData{
					{
						TNF:        0x01,
						Type:       []byte("T"),
						Payload:    MakeTextRecordPayload("Hello", "en"),
						RecordType: "text",
					},
					{
						TNF:        0x01,
						Type:       []byte("U"),
						Payload:    MakeURIRecordPayload("https://example.com"),
						RecordType: "uri",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			input:   nil,
			wantErr: true,
		},
		{
			name: "empty records",
			input: &NDEFMessageData{
				Records: []NDEFRecordData{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ConvertNDEFMessageData(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertNDEFMessageData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && msg == nil {
				t.Error("ConvertNDEFMessageData() returned nil message")
			}
		})
	}
}

func TestConvertSmartphoneTagData(t *testing.T) {
	tests := []struct {
		name    string
		input   SmartphoneTagData
		wantErr bool
	}{
		{
			name: "valid tag with NDEF",
			input: SmartphoneTagData{
				DeviceID:   "device-123",
				UID:        "04:AB:CD:EF",
				Technology: "ISO14443A",
				Type:       "MIFARE Classic 1K",
				ScannedAt:  time.Now(),
				NDEFMessage: &NDEFMessageData{
					Records: []NDEFRecordData{
						{
							TNF:        0x01,
							Type:       []byte("T"),
							Payload:    MakeTextRecordPayload("Test", "en"),
							RecordType: "text",
							Content:    "Test",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid tag without NDEF",
			input: SmartphoneTagData{
				DeviceID:   "device-123",
				UID:        "04ABCDEF",
				Technology: "ISO14443A",
				Type:       "MIFARE Classic 1K",
				ScannedAt:  time.Now(),
				RawData:    []byte("raw data"),
			},
			wantErr: false,
		},
		{
			name: "missing UID",
			input: SmartphoneTagData{
				DeviceID:   "device-123",
				Technology: "ISO14443A",
				Type:       "MIFARE Classic 1K",
			},
			wantErr: true,
		},
		{
			name: "missing technology",
			input: SmartphoneTagData{
				DeviceID: "device-123",
				UID:      "04:AB:CD:EF",
				Type:     "MIFARE Classic 1K",
			},
			wantErr: true,
		},
		{
			name: "invalid UID format",
			input: SmartphoneTagData{
				DeviceID:   "device-123",
				UID:        "invalid",
				Technology: "ISO14443A",
				Type:       "MIFARE Classic 1K",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag, err := ConvertSmartphoneTagData(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertSmartphoneTagData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tag == nil {
					t.Error("ConvertSmartphoneTagData() returned nil tag")
				} else {
					// Verify tag fields
					if tag.UID() != tt.input.UID {
						// UID might be normalized
						normalizedUID, _ := parseSmartphoneUID(tt.input.UID)
						if tag.UID() != normalizedUID {
							t.Errorf("Tag UID = %v, want %v", tag.UID(), normalizedUID)
						}
					}
				}
			}
		})
	}
}
