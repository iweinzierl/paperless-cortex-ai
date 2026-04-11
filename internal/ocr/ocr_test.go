package ocr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedPDFTextLooksUsableRejectsFragmentedOCRLayer(t *testing.T) {
	text := "BERESA\nee\n \nRe\nIn\nWorten\n \nI\nIn\n \ndiesem\n \nBetrag\n \nsind\n \n%\n \nMwSt.\n \nenthalten.\nvon\n \nHISE\n \nDsac\n \nim\n \nLoh\n \n(Kent)\nTaxifahrt/Krankenfahrt\n \nBe:\ndankend\n \nerhalten\n \n;\n \nZY\n \n7\nOsnabrück,den\n \n22\n \niA\n \nOD\n \n2026\n \n_\n \n4\nUnterschrift"
	if embeddedPDFTextLooksUsable(text) {
		t.Fatalf("expected fragmented OCR layer to be rejected, got accepted text: %q", text)
	}
}

func TestEmbeddedPDFTextLooksUsableRejectsControlCharacterGarbage(t *testing.T) {
	text := "<,\x1f\x05<JNS_NJWQ\n\x16Ñ3FHMWNHMY\n&SIWJ\x058YFWRFSS\x05\r'FZ\n1\n\x17\x19\x0e\x05!FSIWJ\x13XYFWRFSS%GFZ\n1\n\x17\x19\x13IJ#)T\x13\x11\x05"
	if embeddedPDFTextLooksUsable(text) {
		t.Fatalf("expected control-character garbage to be rejected, got accepted text: %q", text)
	}
}

func TestEmbeddedPDFTextLooksUsableAcceptsCoherentOCRLayer(t *testing.T) {
	text := "Mobilfunk-Rechnung für März 2026\nGutenTagIngoWeinzierl,\nheuteerhaltenSieIhreMobilfunk-Rechnung.\nRechnungsübersicht (Details siehe Folgeseiten)USt.Betrag\nLeistungen der Telekom Deutschland GmbH\nGrundpreise36,85 €19 %\nRechnungsbetrag36,85 €(davon +19 % USt. auf 30,97 €= 5,88 €)\nDenBetragvon36,85€buchenwiram20.04.2026ab.\nKonto:IBANDE203701900010111XXXXX.ZumSchutzIhrerDatengebenwirdieIBANverkürztan.\nWirbeziehenunsaufdieMandatsreferenzDE000205000600000000000000013587663."
	if !embeddedPDFTextLooksUsable(text) {
		t.Fatalf("expected coherent OCR layer to be accepted, got rejected text: %q", text)
	}
}

func TestBuildScreeningMessageDetectsImageWithoutExtension(t *testing.T) {
	documentPath := filepath.Join(t.TempDir(), "download")
	if err := os.WriteFile(documentPath, []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c,
		0x02, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
		0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
		0x03, 0x03, 0x01, 0xff, 0xa5, 0xfe, 0xff, 0x9f,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
		0xae, 0x42, 0x60, 0x82,
	}, 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	message, err := BuildScreeningMessage(documentPath, DefaultPrompt)
	if err != nil {
		t.Fatalf("build screening message: %v", err)
	}
	if len(message.Images) != 1 {
		t.Fatalf("expected one embedded image, got %d", len(message.Images))
	}
}

func TestBuildScreeningMessageDetectsPDFWithoutExtension(t *testing.T) {
	documentPath := filepath.Join(t.TempDir(), "download")
	if err := os.WriteFile(documentPath, []byte("%PDF-1.4\nthis is not a valid full pdf"), 0o644); err != nil {
		t.Fatalf("write pdf fixture: %v", err)
	}

	_, err := BuildScreeningMessage(documentPath, DefaultPrompt)
	if err == nil {
		t.Fatal("expected PDF parsing error for signature-detected PDF")
	}
	if !strings.Contains(err.Error(), "PDF") && !strings.Contains(err.Error(), "pdf") {
		t.Fatalf("expected PDF-related error, got %v", err)
	}
}

func TestBuildVisionScreeningMessageDetectsImageWithoutExtension(t *testing.T) {
	documentPath := filepath.Join(t.TempDir(), "download")
	if err := os.WriteFile(documentPath, []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c,
		0x02, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
		0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
		0x03, 0x03, 0x01, 0xff, 0xa5, 0xfe, 0xff, 0x9f,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
		0xae, 0x42, 0x60, 0x82,
	}, 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	message, err := BuildVisionScreeningMessage(documentPath, VisionPrompt, 1)
	if err != nil {
		t.Fatalf("build vision screening message: %v", err)
	}
	if len(message.Images) != 1 {
		t.Fatalf("expected one embedded image for vision message, got %d", len(message.Images))
	}
}
