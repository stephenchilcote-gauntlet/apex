package settlement

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moov-io/imagecashletter"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

func writeImageFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeAndParse calls writeX9File and parses the result back.
func writeAndParse(t *testing.T, batchID string, bizDate time.Time, items []x9Item) *imagecashletter.File {
	t.Helper()
	outDir := t.TempDir()
	path := filepath.Join(outDir, "test.x9")

	if err := writeX9File(path, batchID, bizDate, items); err != nil {
		t.Fatalf("writeX9File: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	reader := imagecashletter.NewReader(f, imagecashletter.ReadVariableLineLengthOption())
	iclFile, err := reader.Read()
	if err != nil {
		t.Fatalf("parse X9 file: %v", err)
	}
	return &iclFile
}

// A fixed batch ID (UUID without hyphens = 32 chars, so [:10] and [:8] are safe).
const testBatchID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

var testBizDate = time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// TestWriteX9File_FullStructure — record counts, structure, field-level checks
// ---------------------------------------------------------------------------

func TestWriteX9File_FullStructure(t *testing.T) {
	imgDir := t.TempDir()
	front1 := writeImageFile(t, imgDir, "front1.jpg", []byte("FRONT-IMAGE-ITEM-1"))
	back1 := writeImageFile(t, imgDir, "back1.jpg", []byte("BACK-IMAGE-ITEM-1"))
	front2 := writeImageFile(t, imgDir, "front2.jpg", []byte("FRONT-IMAGE-ITEM-2-LONGER"))
	back2 := writeImageFile(t, imgDir, "back2.jpg", []byte("BACK-IMAGE-ITEM-2-LONGER-STILL"))

	items := []x9Item{
		{
			SequenceNumber: 1, TransferID: "txn-001", AmountCents: 12500,
			MICR:           micrData{RoutingNumber: "123456789", AccountNumber: "11112222", CheckNumber: "9001"},
			FrontImagePath: &front1, BackImagePath: &back1,
		},
		{
			SequenceNumber: 2, TransferID: "txn-002", AmountCents: 25000,
			MICR:           micrData{RoutingNumber: "876543219", AccountNumber: "33334444", CheckNumber: "9002"},
			FrontImagePath: &front2, BackImagePath: &back2,
		},
	}

	icl := writeAndParse(t, testBatchID, testBizDate, items)

	// --- File Header (01) ---
	fh := icl.Header
	assertEqual(t, "FileHeader.ImmediateOrigin", fh.ImmediateOrigin, originRoutingNumber)
	assertEqual(t, "FileHeader.ImmediateDestination", fh.ImmediateDestination, destinationRoutingNumber)
	assertEqual(t, "FileHeader.ImmediateOriginName", fh.ImmediateOriginName, originName)
	assertEqual(t, "FileHeader.ImmediateDestinationName", fh.ImmediateDestinationName, destinationName)
	assertEqual(t, "FileHeader.StandardLevel", fh.StandardLevel, "03")
	assertEqual(t, "FileHeader.TestFileIndicator", fh.TestFileIndicator, "T")
	assertEqual(t, "FileHeader.ResendIndicator", fh.ResendIndicator, "N")
	assertEqual(t, "FileHeader.CountryCode", fh.CountryCode, "US")

	// --- Exactly 1 CashLetter ---
	if len(icl.CashLetters) != 1 {
		t.Fatalf("CashLetterCount = %d, want 1", len(icl.CashLetters))
	}
	cl := icl.CashLetters[0]

	// --- CashLetter Header (10) ---
	clh := cl.CashLetterHeader
	assertEqual(t, "CLH.CollectionTypeIndicator", clh.CollectionTypeIndicator, "01")
	assertEqual(t, "CLH.DestinationRoutingNumber", clh.DestinationRoutingNumber, destinationRoutingNumber)
	assertEqual(t, "CLH.ECEInstitutionRoutingNumber", clh.ECEInstitutionRoutingNumber, originRoutingNumber)
	assertEqual(t, "CLH.RecordTypeIndicator", clh.RecordTypeIndicator, "I")
	assertEqual(t, "CLH.DocumentationTypeIndicator", clh.DocumentationTypeIndicator, "G")
	if clh.CashLetterBusinessDate.Format("2006-01-02") != testBizDate.Format("2006-01-02") {
		t.Errorf("CLH.CashLetterBusinessDate = %s, want %s",
			clh.CashLetterBusinessDate.Format("2006-01-02"), testBizDate.Format("2006-01-02"))
	}
	// CashLetterID derived from batchID (first 8 chars of hyphens-stripped UUID)
	expectedCLID := "a1b2c3d4"
	assertEqual(t, "CLH.CashLetterID", clh.CashLetterID, expectedCLID)

	// --- Exactly 1 Bundle ---
	bundles := cl.GetBundles()
	if len(bundles) != 1 {
		t.Fatalf("BundleCount = %d, want 1", len(bundles))
	}
	b := bundles[0]

	// --- BundleHeader (20) ---
	bh := b.BundleHeader
	assertEqual(t, "BH.CollectionTypeIndicator", bh.CollectionTypeIndicator, "01")
	assertEqual(t, "BH.DestinationRoutingNumber", bh.DestinationRoutingNumber, destinationRoutingNumber)
	assertEqual(t, "BH.ECEInstitutionRoutingNumber", bh.ECEInstitutionRoutingNumber, originRoutingNumber)
	assertEqual(t, "BH.BundleSequenceNumber", bh.BundleSequenceNumber, "0001")
	expectedBundleID := "a1b2c3d4e5"
	assertEqual(t, "BH.BundleID", bh.BundleID, expectedBundleID)
	if bh.BundleBusinessDate.Format("2006-01-02") != testBizDate.Format("2006-01-02") {
		t.Errorf("BH.BundleBusinessDate = %s, want %s",
			bh.BundleBusinessDate.Format("2006-01-02"), testBizDate.Format("2006-01-02"))
	}

	// --- Exactly 2 CheckDetails ---
	checks := b.GetChecks()
	if len(checks) != 2 {
		t.Fatalf("CheckDetailCount = %d, want 2", len(checks))
	}

	// --- CheckDetail (25) per item ---
	for i, cd := range checks {
		item := items[i]
		prefix := "Check[%d]"

		// Amount
		if cd.ItemAmount != int(item.AmountCents) {
			t.Errorf(prefix+".ItemAmount = %d, want %d", i, cd.ItemAmount, int(item.AmountCents))
		}

		// MICR routing split
		wantRouting8 := item.MICR.RoutingNumber[:8]
		wantDigit := item.MICR.RoutingNumber[8:9]
		if cd.PayorBankRoutingNumber != wantRouting8 {
			t.Errorf(prefix+".PayorBankRoutingNumber = %q, want %q", i, cd.PayorBankRoutingNumber, wantRouting8)
		}
		if cd.PayorBankCheckDigit != wantDigit {
			t.Errorf(prefix+".PayorBankCheckDigit = %q, want %q", i, cd.PayorBankCheckDigit, wantDigit)
		}

		// OnUs
		wantOnUs := item.MICR.CheckNumber + "/" + item.MICR.AccountNumber
		if cd.OnUs != wantOnUs {
			t.Errorf(prefix+".OnUs = %q, want %q", i, cd.OnUs, wantOnUs)
		}

		// Metadata fields
		assertEqual(t, "DocumentationTypeIndicator", cd.DocumentationTypeIndicator, "G")
		if cd.MICRValidIndicator != 1 {
			t.Errorf(prefix+".MICRValidIndicator = %d, want 1", i, cd.MICRValidIndicator)
		}
		assertEqual(t, "BOFDIndicator", cd.BOFDIndicator, "Y")
		if cd.AddendumCount != 1 {
			t.Errorf(prefix+".AddendumCount = %d, want 1", i, cd.AddendumCount)
		}
		assertEqual(t, "ArchiveTypeIndicator", cd.ArchiveTypeIndicator, "B")

		// --- CheckDetailAddendumA (26) ---
		addA := cd.GetCheckDetailAddendumA()
		if len(addA) != 1 {
			t.Fatalf(prefix+".AddendumA count = %d, want 1", i, len(addA))
		}
		if addA[0].RecordNumber != 1 {
			t.Errorf(prefix+".AddendumA.RecordNumber = %d, want 1", i, addA[0].RecordNumber)
		}
		assertEqual(t, "AddendumA.ReturnLocationRoutingNumber", addA[0].ReturnLocationRoutingNumber, originRoutingNumber)
		assertEqual(t, "AddendumA.TruncationIndicator", addA[0].TruncationIndicator, "Y")
		assertEqual(t, "AddendumA.BOFDConversionIndicator", addA[0].BOFDConversionIndicator, "2")

		// --- ImageViewDetail (50) and ImageViewData (52): 2 of each ---
		ivds := cd.GetImageViewDetail()
		ivDatas := cd.GetImageViewData()
		if len(ivds) != 2 {
			t.Fatalf(prefix+".ImageViewDetail count = %d, want 2", i, len(ivds))
		}
		if len(ivDatas) != 2 {
			t.Fatalf(prefix+".ImageViewData count = %d, want 2", i, len(ivDatas))
		}

		// Front image (index 0)
		if ivds[0].ViewSideIndicator != 0 {
			t.Errorf(prefix+".IVD[0].ViewSideIndicator = %d, want 0 (front)", i, ivds[0].ViewSideIndicator)
		}
		if ivds[0].ImageIndicator != 1 {
			t.Errorf(prefix+".IVD[0].ImageIndicator = %d, want 1", i, ivds[0].ImageIndicator)
		}
		assertEqual(t, "IVD[0].ImageCreatorRoutingNumber", ivds[0].ImageCreatorRoutingNumber, originRoutingNumber)
		assertEqual(t, "IVD[0].ImageViewFormatIndicator", ivds[0].ImageViewFormatIndicator, "00")
		assertEqual(t, "IVD[0].ImageViewCompressionAlgorithm", ivds[0].ImageViewCompressionAlgorithm, "00")

		// Back image (index 1)
		if ivds[1].ViewSideIndicator != 1 {
			t.Errorf(prefix+".IVD[1].ViewSideIndicator = %d, want 1 (back)", i, ivds[1].ViewSideIndicator)
		}

		// ImageViewData routing + business date
		for j, ivd := range ivDatas {
			assertEqual(t, "IVData.EceInstitutionRoutingNumber", ivd.EceInstitutionRoutingNumber, originRoutingNumber)
			if ivd.BundleBusinessDate.Format("2006-01-02") != testBizDate.Format("2006-01-02") {
				t.Errorf(prefix+".IVData[%d].BundleBusinessDate = %s, want %s",
					i, j, ivd.BundleBusinessDate.Format("2006-01-02"), testBizDate.Format("2006-01-02"))
			}
		}
	}

	// --- Verify embedded image bytes round-trip ---
	checkImageRoundTrip(t, checks[0], "FRONT-IMAGE-ITEM-1", "BACK-IMAGE-ITEM-1")
	checkImageRoundTrip(t, checks[1], "FRONT-IMAGE-ITEM-2-LONGER", "BACK-IMAGE-ITEM-2-LONGER-STILL")

	// --- Control record totals ---
	verifyControlTotals(t, icl, 2, 37500, 4)
}

func checkImageRoundTrip(t *testing.T, cd *imagecashletter.CheckDetail, wantFront, wantBack string) {
	t.Helper()
	ivDatas := cd.GetImageViewData()
	if len(ivDatas) < 2 {
		t.Fatal("expected 2 ImageViewData records")
	}
	if !bytes.Equal(ivDatas[0].ImageData, []byte(wantFront)) {
		t.Errorf("front image data = %q, want %q", string(ivDatas[0].ImageData), wantFront)
	}
	if !bytes.Equal(ivDatas[1].ImageData, []byte(wantBack)) {
		t.Errorf("back image data = %q, want %q", string(ivDatas[1].ImageData), wantBack)
	}
}

func verifyControlTotals(t *testing.T, icl *imagecashletter.File, wantItems int, wantAmount int, wantImages int) {
	t.Helper()

	// BundleControl (70)
	bc := icl.CashLetters[0].GetBundles()[0].BundleControl
	if bc.BundleItemsCount != wantItems {
		t.Errorf("BundleControl.ItemsCount = %d, want %d", bc.BundleItemsCount, wantItems)
	}
	if bc.BundleTotalAmount != wantAmount {
		t.Errorf("BundleControl.TotalAmount = %d, want %d", bc.BundleTotalAmount, wantAmount)
	}
	if bc.MICRValidTotalAmount != wantAmount {
		t.Errorf("BundleControl.MICRValidTotalAmount = %d, want %d", bc.MICRValidTotalAmount, wantAmount)
	}
	if bc.BundleImagesCount != wantImages {
		t.Errorf("BundleControl.ImagesCount = %d, want %d", bc.BundleImagesCount, wantImages)
	}

	// CashLetterControl (90)
	clc := icl.CashLetters[0].CashLetterControl
	if clc.CashLetterBundleCount != 1 {
		t.Errorf("CashLetterControl.BundleCount = %d, want 1", clc.CashLetterBundleCount)
	}
	if clc.CashLetterItemsCount != wantItems {
		t.Errorf("CashLetterControl.ItemsCount = %d, want %d", clc.CashLetterItemsCount, wantItems)
	}
	if clc.CashLetterTotalAmount != wantAmount {
		t.Errorf("CashLetterControl.TotalAmount = %d, want %d", clc.CashLetterTotalAmount, wantAmount)
	}
	if clc.CashLetterImagesCount != wantImages {
		t.Errorf("CashLetterControl.ImagesCount = %d, want %d", clc.CashLetterImagesCount, wantImages)
	}

	// FileControl (99)
	fc := icl.Control
	if fc.CashLetterCount != 1 {
		t.Errorf("FileControl.CashLetterCount = %d, want 1", fc.CashLetterCount)
	}
	if fc.TotalItemCount != wantItems {
		t.Errorf("FileControl.TotalItemCount = %d, want %d", fc.TotalItemCount, wantItems)
	}
	if fc.FileTotalAmount != wantAmount {
		t.Errorf("FileControl.FileTotalAmount = %d, want %d", fc.FileTotalAmount, wantAmount)
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

// ---------------------------------------------------------------------------
// TestWriteX9File_MICRMapping — table-driven MICR routing/OnUs mapping
// ---------------------------------------------------------------------------

func TestWriteX9File_MICRMapping(t *testing.T) {
	imgDir := t.TempDir()
	front := writeImageFile(t, imgDir, "f.jpg", []byte("f"))
	back := writeImageFile(t, imgDir, "b.jpg", []byte("b"))

	cases := []struct {
		name          string
		routing       string
		account       string
		check         string
		wantRouting8  string
		wantDigit     string
		wantOnUs      string
	}{
		{
			name: "9-digit routing with check number",
			routing: "123456789", account: "00012345", check: "9876",
			wantRouting8: "12345678", wantDigit: "9", wantOnUs: "9876/00012345",
		},
		{
			name: "9-digit routing without check number",
			routing: "123456789", account: "00012345", check: "",
			wantRouting8: "12345678", wantDigit: "9", wantOnUs: "00012345",
		},
		{
			name: "8-digit routing with check number",
			routing: "12345678", account: "00012345", check: "9876",
			wantRouting8: "12345678", wantDigit: "0", wantOnUs: "9876/00012345",
		},
		{
			name: "8-digit routing without check number",
			routing: "12345678", account: "ACCT-555", check: "",
			wantRouting8: "12345678", wantDigit: "0", wantOnUs: "ACCT-555",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := []x9Item{{
				SequenceNumber: 1, TransferID: "txn-micr", AmountCents: 100,
				MICR:           micrData{RoutingNumber: tc.routing, AccountNumber: tc.account, CheckNumber: tc.check},
				FrontImagePath: &front, BackImagePath: &back,
			}}
			icl := writeAndParse(t, testBatchID, testBizDate, items)

			cd := icl.CashLetters[0].GetBundles()[0].GetChecks()[0]
			if cd.PayorBankRoutingNumber != tc.wantRouting8 {
				t.Errorf("PayorBankRoutingNumber = %q, want %q", cd.PayorBankRoutingNumber, tc.wantRouting8)
			}
			if cd.PayorBankCheckDigit != tc.wantDigit {
				t.Errorf("PayorBankCheckDigit = %q, want %q", cd.PayorBankCheckDigit, tc.wantDigit)
			}
			if cd.OnUs != tc.wantOnUs {
				t.Errorf("OnUs = %q, want %q", cd.OnUs, tc.wantOnUs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestWriteX9File_ImageEmbedding — front-only, back-only, both, neither
// ---------------------------------------------------------------------------

func TestWriteX9File_ImageEmbedding(t *testing.T) {
	imgDir := t.TempDir()
	frontPath := writeImageFile(t, imgDir, "front.dat", []byte("FRONT-BYTES-HERE"))
	backPath := writeImageFile(t, imgDir, "back.dat", []byte("BACK-BYTES-HERE"))

	baseItem := func() x9Item {
		return x9Item{
			SequenceNumber: 1, TransferID: "txn-img", AmountCents: 500,
			MICR: micrData{RoutingNumber: "111000025", AccountNumber: "99887766", CheckNumber: "0001"},
		}
	}

	t.Run("both images", func(t *testing.T) {
		item := baseItem()
		item.FrontImagePath = &frontPath
		item.BackImagePath = &backPath
		icl := writeAndParse(t, testBatchID, testBizDate, []x9Item{item})

		cd := icl.CashLetters[0].GetBundles()[0].GetChecks()[0]
		ivds := cd.GetImageViewDetail()
		ivDatas := cd.GetImageViewData()
		if len(ivds) != 2 {
			t.Fatalf("IVD count = %d, want 2", len(ivds))
		}
		if len(ivDatas) != 2 {
			t.Fatalf("IVData count = %d, want 2", len(ivDatas))
		}
		if ivds[0].ViewSideIndicator != 0 {
			t.Errorf("front ViewSideIndicator = %d, want 0", ivds[0].ViewSideIndicator)
		}
		if ivds[1].ViewSideIndicator != 1 {
			t.Errorf("back ViewSideIndicator = %d, want 1", ivds[1].ViewSideIndicator)
		}
		if !bytes.Equal(ivDatas[0].ImageData, []byte("FRONT-BYTES-HERE")) {
			t.Errorf("front image data mismatch")
		}
		if !bytes.Equal(ivDatas[1].ImageData, []byte("BACK-BYTES-HERE")) {
			t.Errorf("back image data mismatch")
		}

		verifyControlTotals(t, icl, 1, 500, 2)
	})

	t.Run("front only", func(t *testing.T) {
		item := baseItem()
		item.FrontImagePath = &frontPath
		item.BackImagePath = nil
		icl := writeAndParse(t, testBatchID, testBizDate, []x9Item{item})

		cd := icl.CashLetters[0].GetBundles()[0].GetChecks()[0]
		ivds := cd.GetImageViewDetail()
		ivDatas := cd.GetImageViewData()
		if len(ivds) != 1 {
			t.Fatalf("IVD count = %d, want 1", len(ivds))
		}
		if ivds[0].ViewSideIndicator != 0 {
			t.Errorf("ViewSideIndicator = %d, want 0 (front)", ivds[0].ViewSideIndicator)
		}
		if !bytes.Equal(ivDatas[0].ImageData, []byte("FRONT-BYTES-HERE")) {
			t.Errorf("front image data mismatch")
		}

		verifyControlTotals(t, icl, 1, 500, 1)
	})

	t.Run("back only", func(t *testing.T) {
		item := baseItem()
		item.FrontImagePath = nil
		item.BackImagePath = &backPath
		icl := writeAndParse(t, testBatchID, testBizDate, []x9Item{item})

		cd := icl.CashLetters[0].GetBundles()[0].GetChecks()[0]
		ivds := cd.GetImageViewDetail()
		ivDatas := cd.GetImageViewData()
		if len(ivds) != 1 {
			t.Fatalf("IVD count = %d, want 1", len(ivds))
		}
		if ivds[0].ViewSideIndicator != 1 {
			t.Errorf("ViewSideIndicator = %d, want 1 (back)", ivds[0].ViewSideIndicator)
		}
		if !bytes.Equal(ivDatas[0].ImageData, []byte("BACK-BYTES-HERE")) {
			t.Errorf("back image data mismatch")
		}

		verifyControlTotals(t, icl, 1, 500, 1)
	})

	t.Run("no images", func(t *testing.T) {
		item := baseItem()
		item.FrontImagePath = nil
		item.BackImagePath = nil
		icl := writeAndParse(t, testBatchID, testBizDate, []x9Item{item})

		cd := icl.CashLetters[0].GetBundles()[0].GetChecks()[0]
		ivds := cd.GetImageViewDetail()
		ivDatas := cd.GetImageViewData()
		if len(ivds) != 0 {
			t.Errorf("IVD count = %d, want 0", len(ivds))
		}
		if len(ivDatas) != 0 {
			t.Errorf("IVData count = %d, want 0", len(ivDatas))
		}
		if cd.ItemAmount != 500 {
			t.Errorf("ItemAmount = %d, want 500", cd.ItemAmount)
		}

		verifyControlTotals(t, icl, 1, 500, 0)
	})
}

// ---------------------------------------------------------------------------
// TestWriteX9File_ControlTotals — arithmetic correctness at all levels
// ---------------------------------------------------------------------------

func TestWriteX9File_ControlTotals(t *testing.T) {
	imgDir := t.TempDir()
	front := writeImageFile(t, imgDir, "f.jpg", []byte("img"))
	back := writeImageFile(t, imgDir, "b.jpg", []byte("img"))

	t.Run("single item", func(t *testing.T) {
		items := []x9Item{{
			SequenceNumber: 1, TransferID: "t-1", AmountCents: 12345,
			MICR:           micrData{RoutingNumber: "111000025", AccountNumber: "111", CheckNumber: "1"},
			FrontImagePath: &front, BackImagePath: &back,
		}}
		icl := writeAndParse(t, testBatchID, testBizDate, items)
		verifyControlTotals(t, icl, 1, 12345, 2)
	})

	t.Run("three items", func(t *testing.T) {
		items := []x9Item{
			{SequenceNumber: 1, TransferID: "t-1", AmountCents: 1250,
				MICR: micrData{RoutingNumber: "111000025", AccountNumber: "a", CheckNumber: "1"},
				FrontImagePath: &front, BackImagePath: &back},
			{SequenceNumber: 2, TransferID: "t-2", AmountCents: 2500,
				MICR: micrData{RoutingNumber: "111000025", AccountNumber: "b", CheckNumber: "2"},
				FrontImagePath: &front, BackImagePath: &back},
			{SequenceNumber: 3, TransferID: "t-3", AmountCents: 9999,
				MICR: micrData{RoutingNumber: "111000025", AccountNumber: "c", CheckNumber: "3"},
				FrontImagePath: &front, BackImagePath: &back},
		}
		icl := writeAndParse(t, testBatchID, testBizDate, items)
		verifyControlTotals(t, icl, 3, 13749, 6)
	})

	t.Run("large amount", func(t *testing.T) {
		items := []x9Item{{
			SequenceNumber: 1, TransferID: "t-big", AmountCents: 99999999,
			MICR:           micrData{RoutingNumber: "111000025", AccountNumber: "big", CheckNumber: "1"},
			FrontImagePath: &front, BackImagePath: &back,
		}}
		icl := writeAndParse(t, testBatchID, testBizDate, items)
		verifyControlTotals(t, icl, 1, 99999999, 2)
	})

	t.Run("mixed images affects image count only", func(t *testing.T) {
		items := []x9Item{
			{SequenceNumber: 1, TransferID: "t-1", AmountCents: 1000,
				MICR: micrData{RoutingNumber: "111000025", AccountNumber: "a", CheckNumber: "1"},
				FrontImagePath: &front, BackImagePath: &back},
			{SequenceNumber: 2, TransferID: "t-2", AmountCents: 2000,
				MICR: micrData{RoutingNumber: "111000025", AccountNumber: "b", CheckNumber: "2"},
				FrontImagePath: &front, BackImagePath: nil},
			{SequenceNumber: 3, TransferID: "t-3", AmountCents: 3000,
				MICR: micrData{RoutingNumber: "111000025", AccountNumber: "c", CheckNumber: "3"},
				FrontImagePath: nil, BackImagePath: nil},
		}
		icl := writeAndParse(t, testBatchID, testBizDate, items)
		// 3 items, total 6000, images: 2 + 1 + 0 = 3
		verifyControlTotals(t, icl, 3, 6000, 3)
	})
}

// ---------------------------------------------------------------------------
// TestWriteX9File_RoundTripFidelity — write, parse, compare field by field
// ---------------------------------------------------------------------------

func TestWriteX9File_RoundTripFidelity(t *testing.T) {
	imgDir := t.TempDir()

	type roundTripExpectation struct {
		amount       int
		routing8     string
		checkDigit   string
		onUs         string
		frontData    string
		backData     string
	}

	expectations := []roundTripExpectation{
		{amount: 5555, routing8: "11100002", checkDigit: "5", onUs: "0042/ACCT-A", frontData: "front-aaa", backData: "back-aaa"},
		{amount: 7777, routing8: "23138010", checkDigit: "4", onUs: "0099/ACCT-B", frontData: "front-bbb", backData: "back-bbb"},
		{amount: 100, routing8: "99900011", checkDigit: "2", onUs: "ACCT-C", frontData: "front-ccc", backData: "back-ccc"},
	}

	var items []x9Item
	for i, e := range expectations {
		fp := writeImageFile(t, imgDir, e.frontData+".jpg", []byte(e.frontData))
		bp := writeImageFile(t, imgDir, e.backData+".jpg", []byte(e.backData))
		routing := e.routing8 + e.checkDigit
		check := ""
		// Reverse-engineer OnUs to get check number
		if idx := len(e.onUs) - len(e.onUs); idx >= 0 {
			// Parse "check/account" or "account"
			parts := splitOnUs(e.onUs)
			check = parts.check
		}
		items = append(items, x9Item{
			SequenceNumber: i + 1, TransferID: "rt-" + e.routing8[:4],
			AmountCents: int64(e.amount),
			MICR:        micrData{RoutingNumber: routing, AccountNumber: splitOnUs(e.onUs).account, CheckNumber: check},
			FrontImagePath: &fp, BackImagePath: &bp,
		})
	}

	icl := writeAndParse(t, testBatchID, testBizDate, items)
	checks := icl.CashLetters[0].GetBundles()[0].GetChecks()
	if len(checks) != len(expectations) {
		t.Fatalf("checks = %d, want %d", len(checks), len(expectations))
	}

	for i, e := range expectations {
		cd := checks[i]
		if cd.ItemAmount != e.amount {
			t.Errorf("[%d] amount = %d, want %d", i, cd.ItemAmount, e.amount)
		}
		if cd.PayorBankRoutingNumber != e.routing8 {
			t.Errorf("[%d] routing8 = %q, want %q", i, cd.PayorBankRoutingNumber, e.routing8)
		}
		if cd.PayorBankCheckDigit != e.checkDigit {
			t.Errorf("[%d] checkDigit = %q, want %q", i, cd.PayorBankCheckDigit, e.checkDigit)
		}
		if cd.OnUs != e.onUs {
			t.Errorf("[%d] OnUs = %q, want %q", i, cd.OnUs, e.onUs)
		}

		ivDatas := cd.GetImageViewData()
		if len(ivDatas) != 2 {
			t.Fatalf("[%d] IVData count = %d, want 2", i, len(ivDatas))
		}
		if !bytes.Equal(ivDatas[0].ImageData, []byte(e.frontData)) {
			t.Errorf("[%d] front image mismatch: got %q", i, string(ivDatas[0].ImageData))
		}
		if !bytes.Equal(ivDatas[1].ImageData, []byte(e.backData)) {
			t.Errorf("[%d] back image mismatch: got %q", i, string(ivDatas[1].ImageData))
		}
	}

	// Verify total across all items
	var totalAmount int
	for _, e := range expectations {
		totalAmount += e.amount
	}
	verifyControlTotals(t, icl, len(expectations), totalAmount, len(expectations)*2)
}

type onUsParts struct {
	check   string
	account string
}

func splitOnUs(s string) onUsParts {
	for i, c := range s {
		if c == '/' {
			return onUsParts{check: s[:i], account: s[i+1:]}
		}
	}
	return onUsParts{account: s}
}

// ---------------------------------------------------------------------------
// TestWriteX9File_ErrorPaths — missing image files
// ---------------------------------------------------------------------------

func TestWriteX9File_ErrorPaths(t *testing.T) {
	t.Run("missing front image file", func(t *testing.T) {
		badPath := "/nonexistent/path/front.jpg"
		items := []x9Item{{
			SequenceNumber: 1, TransferID: "txn-err", AmountCents: 100,
			MICR:           micrData{RoutingNumber: "111000025", AccountNumber: "123", CheckNumber: "1"},
			FrontImagePath: &badPath, BackImagePath: nil,
		}}
		outDir := t.TempDir()
		path := filepath.Join(outDir, "err.x9")
		err := writeX9File(path, testBatchID, testBizDate, items)
		if err == nil {
			t.Fatal("expected error for missing front image")
		}
		if !containsAny(err.Error(), "front image", "open", "no such file") {
			t.Errorf("error = %q, expected mention of front image / file open failure", err.Error())
		}
	})

	t.Run("missing back image file", func(t *testing.T) {
		imgDir := t.TempDir()
		frontPath := writeImageFile(t, imgDir, "f.jpg", []byte("front"))
		badPath := "/nonexistent/path/back.jpg"
		items := []x9Item{{
			SequenceNumber: 1, TransferID: "txn-err", AmountCents: 100,
			MICR:           micrData{RoutingNumber: "111000025", AccountNumber: "123", CheckNumber: "1"},
			FrontImagePath: &frontPath, BackImagePath: &badPath,
		}}
		outDir := t.TempDir()
		path := filepath.Join(outDir, "err.x9")
		err := writeX9File(path, testBatchID, testBizDate, items)
		if err == nil {
			t.Fatal("expected error for missing back image")
		}
		if !containsAny(err.Error(), "back image", "open", "no such file") {
			t.Errorf("error = %q, expected mention of back image / file open failure", err.Error())
		}
	})
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// TestWriteX9File_LibraryValidation — file passes Validate()
// ---------------------------------------------------------------------------

func TestWriteX9File_LibraryValidation(t *testing.T) {
	imgDir := t.TempDir()
	front := writeImageFile(t, imgDir, "f.jpg", []byte("front-data"))
	back := writeImageFile(t, imgDir, "b.jpg", []byte("back-data"))

	items := []x9Item{
		{SequenceNumber: 1, TransferID: "t-1", AmountCents: 5000,
			MICR: micrData{RoutingNumber: "021000089", AccountNumber: "12345", CheckNumber: "100"},
			FrontImagePath: &front, BackImagePath: &back},
		{SequenceNumber: 2, TransferID: "t-2", AmountCents: 15000,
			MICR: micrData{RoutingNumber: "021000089", AccountNumber: "67890", CheckNumber: "200"},
			FrontImagePath: &front, BackImagePath: &back},
	}

	outDir := t.TempDir()
	path := filepath.Join(outDir, "validate.x9")
	if err := writeX9File(path, testBatchID, testBizDate, items); err != nil {
		t.Fatalf("writeX9File: %v", err)
	}

	// Re-open and validate via the library
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	reader := imagecashletter.NewReader(f, imagecashletter.ReadVariableLineLengthOption())
	iclFile, err := reader.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := iclFile.Validate(); err != nil {
		t.Errorf("Validate() failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestWriteX9File_SingleItem — minimal batch
// ---------------------------------------------------------------------------

func TestWriteX9File_SingleItem(t *testing.T) {
	imgDir := t.TempDir()
	front := writeImageFile(t, imgDir, "f.jpg", []byte("single-front"))
	back := writeImageFile(t, imgDir, "b.jpg", []byte("single-back"))

	items := []x9Item{{
		SequenceNumber: 1, TransferID: "t-single", AmountCents: 42,
		MICR:           micrData{RoutingNumber: "021000089", AccountNumber: "ACCT", CheckNumber: ""},
		FrontImagePath: &front, BackImagePath: &back,
	}}

	icl := writeAndParse(t, testBatchID, testBizDate, items)

	checks := icl.CashLetters[0].GetBundles()[0].GetChecks()
	if len(checks) != 1 {
		t.Fatalf("check count = %d, want 1", len(checks))
	}
	if checks[0].ItemAmount != 42 {
		t.Errorf("amount = %d, want 42", checks[0].ItemAmount)
	}
	// OnUs with empty check number should be just the account
	if checks[0].OnUs != "ACCT" {
		t.Errorf("OnUs = %q, want %q", checks[0].OnUs, "ACCT")
	}

	verifyControlTotals(t, icl, 1, 42, 2)
}

// ---------------------------------------------------------------------------
// TestWriteX9File_BatchIDDerivation — CashLetterID and BundleID from batchID
// ---------------------------------------------------------------------------

func TestWriteX9File_BatchIDDerivation(t *testing.T) {
	imgDir := t.TempDir()
	front := writeImageFile(t, imgDir, "f.jpg", []byte("x"))
	back := writeImageFile(t, imgDir, "b.jpg", []byte("y"))

	batchID := "deadbeef-1234-5678-9abc-def012345678"
	// Stripped: "deadbeef123456789abcdef012345678"
	expectedCLID := "deadbeef" // [:8]
	expectedBID := "deadbeef12"  // [:10]

	items := []x9Item{{
		SequenceNumber: 1, TransferID: "t-id", AmountCents: 100,
		MICR:           micrData{RoutingNumber: "111000025", AccountNumber: "a", CheckNumber: "1"},
		FrontImagePath: &front, BackImagePath: &back,
	}}

	icl := writeAndParse(t, batchID, testBizDate, items)

	clh := icl.CashLetters[0].CashLetterHeader
	bh := icl.CashLetters[0].GetBundles()[0].BundleHeader

	if clh.CashLetterID != expectedCLID {
		t.Errorf("CashLetterID = %q, want %q", clh.CashLetterID, expectedCLID)
	}
	if bh.BundleID != expectedBID {
		t.Errorf("BundleID = %q, want %q", bh.BundleID, expectedBID)
	}
}

// ---------------------------------------------------------------------------
// TestParseX9Items — unit test for the fileItem → x9Item converter
// ---------------------------------------------------------------------------

func TestParseX9Items(t *testing.T) {
	front := "/images/front.jpg"
	back := "/images/back.jpg"

	fileItems := []fileItem{
		{
			SequenceNumber: 1, TransferID: "txn-1", AmountCents: 500,
			MICR:   micrData{RoutingNumber: "111000025", AccountNumber: "ACCT1", CheckNumber: "0001"},
			Images: imageData{FrontPath: &front, BackPath: &back},
		},
		{
			SequenceNumber: 2, TransferID: "txn-2", AmountCents: 1000,
			MICR:   micrData{RoutingNumber: "222000036", AccountNumber: "ACCT2", CheckNumber: ""},
			Images: imageData{FrontPath: nil, BackPath: nil},
		},
	}

	result := parseX9Items(fileItems)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// Item 1
	if result[0].SequenceNumber != 1 || result[0].TransferID != "txn-1" || result[0].AmountCents != 500 {
		t.Errorf("item[0] basic fields mismatch")
	}
	if result[0].MICR.RoutingNumber != "111000025" {
		t.Errorf("item[0] MICR routing = %q", result[0].MICR.RoutingNumber)
	}
	if result[0].FrontImagePath == nil || *result[0].FrontImagePath != front {
		t.Errorf("item[0] front path mismatch")
	}
	if result[0].BackImagePath == nil || *result[0].BackImagePath != back {
		t.Errorf("item[0] back path mismatch")
	}

	// Item 2 — nil images
	if result[1].FrontImagePath != nil {
		t.Errorf("item[1] front path should be nil")
	}
	if result[1].BackImagePath != nil {
		t.Errorf("item[1] back path should be nil")
	}
	if result[1].MICR.CheckNumber != "" {
		t.Errorf("item[1] check number should be empty")
	}
}

// ---------------------------------------------------------------------------
// TestParseMICRSnapshot — unit test for JSON snapshot parser
// ---------------------------------------------------------------------------

func TestParseMICRSnapshot(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		j := `{"routingNumber":"111000025","accountNumber":"ACCT1","checkNumber":"0042"}`
		m := parseMICRSnapshot(&j)
		if m.RoutingNumber != "111000025" || m.AccountNumber != "ACCT1" || m.CheckNumber != "0042" {
			t.Errorf("parsed = %+v", m)
		}
	})

	t.Run("nil input", func(t *testing.T) {
		m := parseMICRSnapshot(nil)
		if m.RoutingNumber != "" || m.AccountNumber != "" || m.CheckNumber != "" {
			t.Errorf("expected empty micrData, got %+v", m)
		}
	})

	t.Run("empty JSON object", func(t *testing.T) {
		j := `{}`
		m := parseMICRSnapshot(&j)
		if m.RoutingNumber != "" || m.AccountNumber != "" || m.CheckNumber != "" {
			t.Errorf("expected empty fields, got %+v", m)
		}
	})
}
