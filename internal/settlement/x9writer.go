package settlement

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/moov-io/imagecashletter"
)

const (
	// Demo institution routing numbers (ACME Brokerage).
	// In production these would come from configuration.
	originRoutingNumber      = "121042882"
	destinationRoutingNumber = "231380104"
	originName               = "ACME Brokerage"
	destinationName          = "Fed Reserve"
)

type x9Item struct {
	SequenceNumber int
	TransferID     string
	AmountCents    int64
	MICR           micrData
	FrontImagePath *string
	BackImagePath  *string
}

func writeX9File(path string, batchID string, businessDate time.Time, items []x9Item) error {
	now := time.Now().UTC()
	alphanumID := strings.ReplaceAll(batchID, "-", "")

	fh := imagecashletter.NewFileHeader()
	fh.StandardLevel = "03"
	fh.TestFileIndicator = "T"
	fh.ImmediateDestination = destinationRoutingNumber
	fh.ImmediateOrigin = originRoutingNumber
	fh.FileCreationDate = now
	fh.FileCreationTime = now
	fh.ResendIndicator = "N"
	fh.ImmediateDestinationName = destinationName
	fh.ImmediateOriginName = originName
	fh.FileIDModifier = ""
	fh.CountryCode = "US"
	fh.CompanionDocumentIndicator = ""

	clh := imagecashletter.NewCashLetterHeader()
	clh.CollectionTypeIndicator = "01"
	clh.DestinationRoutingNumber = destinationRoutingNumber
	clh.ECEInstitutionRoutingNumber = originRoutingNumber
	clh.CashLetterBusinessDate = businessDate
	clh.CashLetterCreationDate = now
	clh.CashLetterCreationTime = now
	clh.RecordTypeIndicator = "I"
	clh.DocumentationTypeIndicator = "G"
	clh.CashLetterID = alphanumID[:8]

	bh := imagecashletter.NewBundleHeader()
	bh.CollectionTypeIndicator = "01"
	bh.DestinationRoutingNumber = destinationRoutingNumber
	bh.ECEInstitutionRoutingNumber = originRoutingNumber
	bh.BundleBusinessDate = businessDate
	bh.BundleCreationDate = now
	bh.BundleID = alphanumID[:10]
	bh.BundleSequenceNumber = "0001"

	bundle := imagecashletter.NewBundle(bh)

	for _, item := range items {
		cd := imagecashletter.NewCheckDetail()

		routing := item.MICR.RoutingNumber
		if len(routing) >= 9 {
			cd.PayorBankRoutingNumber = routing[:8]
			cd.PayorBankCheckDigit = routing[8:9]
		} else if len(routing) == 8 {
			cd.PayorBankRoutingNumber = routing
			cd.PayorBankCheckDigit = "0"
		}

		cd.OnUs = item.MICR.AccountNumber
		if item.MICR.CheckNumber != "" {
			cd.OnUs = item.MICR.CheckNumber + "/" + item.MICR.AccountNumber
		}
		cd.ItemAmount = int(item.AmountCents)
		cd.SetEceInstitutionItemSequenceNumber(item.SequenceNumber)
		cd.DocumentationTypeIndicator = "G"
		cd.MICRValidIndicator = 1
		cd.BOFDIndicator = "Y"
		cd.AddendumCount = 1
		cd.CorrectionIndicator = 0
		cd.ArchiveTypeIndicator = "B"

		addA := imagecashletter.NewCheckDetailAddendumA()
		addA.RecordNumber = 1
		addA.ReturnLocationRoutingNumber = originRoutingNumber
		addA.BOFDEndorsementDate = businessDate
		addA.SetBOFDItemSequenceNumber(item.SequenceNumber)
		addA.TruncationIndicator = "Y"
		addA.BOFDConversionIndicator = "2"
		addA.BOFDCorrectionIndicator = 0
		cd.AddCheckDetailAddendumA(addA)

		seqStr := cd.EceInstitutionItemSequenceNumber

		if item.FrontImagePath != nil {
			frontBytes, err := os.ReadFile(*item.FrontImagePath)
			if err != nil {
				return fmt.Errorf("read front image for %s: %w", item.TransferID, err)
			}
			ivd := imagecashletter.NewImageViewDetail()
			ivd.ImageIndicator = 1
			ivd.ImageCreatorRoutingNumber = originRoutingNumber
			ivd.ImageCreatorDate = now
			ivd.ImageViewFormatIndicator = "00"
			ivd.ImageViewCompressionAlgorithm = "00"
			ivd.ImageViewDataSize = fmt.Sprintf("%d", len(frontBytes))
			ivd.ViewSideIndicator = 0
			ivd.ViewDescriptor = "00"
			ivd.DigitalSignatureIndicator = 0
			cd.AddImageViewDetail(ivd)

			ivData := imagecashletter.NewImageViewData()
			ivData.EceInstitutionRoutingNumber = originRoutingNumber
			ivData.BundleBusinessDate = businessDate
			ivData.EceInstitutionItemSequenceNumber = seqStr
			ivData.LengthImageData = fmt.Sprintf("%d", len(frontBytes))
			ivData.ImageData = frontBytes
			cd.AddImageViewData(ivData)
		}

		if item.BackImagePath != nil {
			backBytes, err := os.ReadFile(*item.BackImagePath)
			if err != nil {
				return fmt.Errorf("read back image for %s: %w", item.TransferID, err)
			}
			ivd := imagecashletter.NewImageViewDetail()
			ivd.ImageIndicator = 1
			ivd.ImageCreatorRoutingNumber = originRoutingNumber
			ivd.ImageCreatorDate = now
			ivd.ImageViewFormatIndicator = "00"
			ivd.ImageViewCompressionAlgorithm = "00"
			ivd.ImageViewDataSize = fmt.Sprintf("%d", len(backBytes))
			ivd.ViewSideIndicator = 1
			ivd.ViewDescriptor = "00"
			ivd.DigitalSignatureIndicator = 0
			cd.AddImageViewDetail(ivd)

			ivData := imagecashletter.NewImageViewData()
			ivData.EceInstitutionRoutingNumber = originRoutingNumber
			ivData.BundleBusinessDate = businessDate
			ivData.EceInstitutionItemSequenceNumber = seqStr
			ivData.LengthImageData = fmt.Sprintf("%d", len(backBytes))
			ivData.ImageData = backBytes
			cd.AddImageViewData(ivData)
		}

		bundle.AddCheckDetail(cd)
	}

	cashLetter := imagecashletter.NewCashLetter(clh)
	cashLetter.AddBundle(bundle)
	if err := cashLetter.Create(); err != nil {
		return fmt.Errorf("create cash letter: %w", err)
	}

	file := imagecashletter.NewFile()
	file.SetHeader(fh)
	file.AddCashLetter(cashLetter)
	if err := file.Create(); err != nil {
		return fmt.Errorf("create ICL file structure: %w", err)
	}
	if err := file.Validate(); err != nil {
		return fmt.Errorf("validate ICL file: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	w := imagecashletter.NewWriter(f, imagecashletter.WriteVariableLineLengthOption())
	if err := w.Write(file); err != nil {
		return fmt.Errorf("write ICL file: %w", err)
	}
	w.Flush()

	return nil
}

func parseX9Items(fileItems []fileItem) []x9Item {
	items := make([]x9Item, len(fileItems))
	for i, fi := range fileItems {
		// Reconstruct MICR from the JSON snapshot stored in batch items
		items[i] = x9Item{
			SequenceNumber: fi.SequenceNumber,
			TransferID:     fi.TransferID,
			AmountCents:    fi.AmountCents,
			MICR:           fi.MICR,
			FrontImagePath: fi.Images.FrontPath,
			BackImagePath:  fi.Images.BackPath,
		}
	}
	return items
}

// parseMICRSnapshot parses the JSON MICR snapshot stored in settlement_batch_items.
func parseMICRSnapshot(jsonStr *string) micrData {
	if jsonStr == nil {
		return micrData{}
	}
	var m micrData
	json.Unmarshal([]byte(*jsonStr), &m)
	return m
}
