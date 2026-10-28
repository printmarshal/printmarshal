package snmp

// Object identifiers used by the standards driver. Everything here is from an
// IETF standard MIB:
//
//   - System group, RFC 3418 (SNMPv2-MIB)           1.3.6.1.2.1.1
//   - Interfaces, RFC 2863 (IF-MIB)                 1.3.6.1.2.1.2
//   - Host Resources MIB, RFC 2790                  1.3.6.1.2.1.25
//   - Printer MIB v2, RFC 3805                      1.3.6.1.2.1.43
//
// No manufacturer enterprise subtree appears here by design (spec §7); vendor
// plugins read their own subtrees.
const (
	OIDSysDescr    = ".1.3.6.1.2.1.1.1.0"
	OIDSysObjectID = ".1.3.6.1.2.1.1.2.0"
	OIDSysUpTime   = ".1.3.6.1.2.1.1.3.0"
	OIDSysContact  = ".1.3.6.1.2.1.1.4.0"
	OIDSysName     = ".1.3.6.1.2.1.1.5.0"
	OIDSysLocation = ".1.3.6.1.2.1.1.6.0"

	OIDIfPhysAddress = ".1.3.6.1.2.1.2.2.1.6"

	// Host Resources MIB (RFC 2790).
	OIDHrDeviceType           = ".1.3.6.1.2.1.25.3.2.1.2"
	OIDHrDeviceDescr          = ".1.3.6.1.2.1.25.3.2.1.3"
	OIDHrDeviceStatus         = ".1.3.6.1.2.1.25.3.2.1.5"
	OIDHrPrinterStatus        = ".1.3.6.1.2.1.25.3.5.1.1"
	OIDHrPrinterDetectedError = ".1.3.6.1.2.1.25.3.5.1.2"
	OIDHrDevicePrinterType    = ".1.3.6.1.2.1.25.3.1.5" // hrDevicePrinter

	// Printer MIB (RFC 3805). prtGeneralTable read-write objects used by write_config.
	OIDPrtInputDefaultIndex   = ".1.3.6.1.2.1.43.5.1.1.6"
	OIDPrtMarkerDefaultIndex  = ".1.3.6.1.2.1.43.5.1.1.8"
	OIDPrtGeneralPrinterName  = ".1.3.6.1.2.1.43.5.1.1.16"
	OIDPrtInputName           = ".1.3.6.1.2.1.43.8.2.1.13"
	OIDPrtGeneralSerialNumber = ".1.3.6.1.2.1.43.5.1.1.17"
	OIDPrtInputTable          = ".1.3.6.1.2.1.43.8.2.1"
	OIDPrtOutputTable         = ".1.3.6.1.2.1.43.9.2.1"
	OIDPrtMarkerTable         = ".1.3.6.1.2.1.43.10.2.1"
	OIDPrtMarkerLifeCount     = ".1.3.6.1.2.1.43.10.2.1.4"
	OIDPrtMarkerPowerOnCount  = ".1.3.6.1.2.1.43.10.2.1.5"
	OIDPrtMarkerSuppliesTable = ".1.3.6.1.2.1.43.11.1.1"
	OIDPrtMarkerColorantValue = ".1.3.6.1.2.1.43.12.1.1.4"
	OIDPrtConsoleDisplayText  = ".1.3.6.1.2.1.43.16.5.1.2"
	OIDPrtAlertTable          = ".1.3.6.1.2.1.43.18.1.1"
)

// Column numbers inside the Printer MIB tables (RFC 3805).
const (
	colSuppliesClass       = 4
	colSuppliesType        = 5
	colSuppliesDescription = 6
	colSuppliesUnit        = 7
	colSuppliesMaxCapacity = 8
	colSuppliesLevel       = 9
	colSuppliesColorant    = 3

	colInputType         = 2
	colInputDimUnit      = 3
	colInputDimFeedDecl  = 4
	colInputDimXFeedDecl = 5
	colInputMaxCapacity  = 9
	colInputCurrentLevel = 10
	colInputStatus       = 11
	colInputMediaName    = 12
	colInputName         = 13
	colInputMediaType    = 21

	colOutputType        = 2
	colOutputMaxCapacity = 4
	colOutputRemaining   = 5
	colOutputStatus      = 6
	colOutputName        = 7

	colAlertSeverity    = 2
	colAlertGroup       = 4
	colAlertGroupIndex  = 5
	colAlertCode        = 7
	colAlertDescription = 8
	colAlertTime        = 9
)

// supplyTypes maps prtMarkerSuppliesType (RFC 3805 PrtMarkerSuppliesTypeTC).
var supplyTypes = map[int]string{
	1: "other", 2: "unknown", 3: "toner", 4: "wasteToner", 5: "ink", 6: "inkCartridge",
	7: "inkRibbon", 8: "wasteInk", 9: "opc", 10: "developer", 11: "fuserOil", 12: "solidWax",
	13: "ribbonWax", 14: "wasteWax", 15: "fuser", 16: "coronaWire", 17: "fuserOilWick",
	18: "cleanerUnit", 19: "fuserCleaningPad", 20: "transferUnit", 21: "tonerCartridge",
	22: "fuserOiler", 23: "water", 24: "wasteWater", 25: "glueWaterAdditive", 26: "wastePaper",
	27: "bindingSupply", 28: "bandingSupply", 29: "stitchingWire", 30: "shrinkWrap",
	31: "paperWrap", 32: "staples", 33: "inserts", 34: "covers",
}

// supplyUnits maps prtMarkerSuppliesSupplyUnit (PrtMarkerSuppliesSupplyUnitTC).
var supplyUnits = map[int]string{
	1: "other", 2: "unknown", 3: "tenThousandthsOfInches", 4: "micrometers", 7: "impressions",
	8: "sheets", 11: "hours", 12: "thousandthsOfOunces", 13: "tenthsOfGrams", 14: "hundrethsOfFluidOunces",
	15: "tenthsOfMilliliters", 16: "feet", 17: "meters", 18: "items", 19: "percent",
}

var inputTypes = map[int]string{
	1: "other", 2: "unknown", 3: "sheetFeedAutoRemovableTray", 4: "sheetFeedAutoNonRemovableTray",
	5: "sheetFeedManual", 6: "continuousRoll", 7: "continuousFanFold",
}

var outputTypes = map[int]string{
	1: "other", 2: "unknown", 3: "removableBin", 4: "unRemovableBin", 5: "continuousRollDevice",
	6: "mailBox", 7: "continuousFanFold",
}

var alertGroups = map[int]string{
	1: "other", 3: "hostResourcesMIBStorageTable", 4: "hostResourcesMIBDeviceTable", 5: "generalPrinter",
	6: "cover", 7: "localization", 8: "input", 9: "output", 10: "marker", 11: "markerSupplies",
	12: "markerColorant", 13: "mediaPath", 14: "channel", 15: "interpreter", 16: "consoleDisplayBuffer",
	17: "consoleLights", 18: "alert", 30: "finDevice", 31: "finSupply", 32: "finSupplyMediaInput", 33: "finAttribute",
}

// alertCodes maps PrtAlertCodeTC to a readable name, used when the device leaves prtAlertDescription empty.
var alertCodes = map[int]string{
	1: "other", 2: "unknown", 3: "coverOpen", 4: "coverClosed", 5: "interlockOpen", 6: "interlockClosed",
	7: "configurationChange", 8: "jam", 9: "subunitMissing", 10: "subunitLifeAlmostOver", 11: "subunitLifeOver",
	12: "subunitAlmostEmpty", 13: "subunitEmpty", 14: "subunitAlmostFull", 15: "subunitFull", 16: "subunitNearLimit",
	17: "subunitAtLimit", 18: "subunitOpened", 19: "subunitClosed", 20: "subunitTurnedOn", 21: "subunitTurnedOff",
	22: "subunitOffline", 23: "subunitPowerSaver", 24: "subunitWarmingUp", 25: "subunitAdded", 26: "subunitRemoved",
	27: "subunitResourceAdded", 28: "subunitResourceRemoved", 29: "subunitRecoverableFailure",
	30: "subunitUnrecoverableFailure", 31: "subunitRecoverableStorageError", 32: "subunitUnrecoverableStorageError",
	33: "subunitMotorFailure", 34: "subunitMemoryExhausted", 35: "subunitUnderTemperature", 36: "subunitOverTemperature",
	37: "subunitTimingFailure", 38: "subunitThermistorFailure",
	501: "doorOpen", 502: "doorClosed", 503: "powerUp", 504: "powerDown", 505: "printerNMSReset",
	506: "printerManualReset", 507: "printerReadyToPrint",
	801: "inputMediaTrayMissing", 802: "inputMediaSizeChange", 803: "inputMediaWeightChange", 804: "inputMediaTypeChange",
	805: "inputMediaColorChange", 806: "inputMediaFormPartsChange", 807: "inputMediaSupplyLow", 808: "inputMediaSupplyEmpty",
	809: "inputMediaChangeRequest", 810: "inputManualInputRequest", 811: "inputTrayPositionFailure",
	812: "inputTrayElevationFailure", 813: "inputCannotFeedSizeSelected",
	901: "outputMediaTrayMissing", 902: "outputMediaTrayAlmostFull", 903: "outputMediaTrayFull", 904: "outputMailboxSelectFailure",
	1001: "markerFuserUnderTemperature", 1002: "markerFuserOverTemperature", 1003: "markerFuserTimingFailure",
	1004: "markerFuserThermistorFailure", 1005: "markerAdjustingPrintQuality",
	1101: "markerTonerEmpty", 1102: "markerInkEmpty", 1103: "markerPrintRibbonEmpty", 1104: "markerTonerAlmostEmpty",
	1105: "markerInkAlmostEmpty", 1106: "markerPrintRibbonAlmostEmpty", 1107: "markerWasteTonerReceptacleAlmostFull",
	1108: "markerWasteInkReceptacleAlmostFull", 1109: "markerWasteTonerReceptacleFull", 1110: "markerWasteInkReceptacleFull",
	1111: "markerOpcLifeAlmostOver", 1112: "markerOpcLifeOver", 1113: "markerDeveloperAlmostEmpty", 1114: "markerDeveloperEmpty",
	1115: "markerTonerCartridgeMissing",
	1301: "mediaPathMediaTrayMissing", 1302: "mediaPathMediaTrayAlmostFull", 1303: "mediaPathMediaTrayFull",
	1304: "mediaPathCannotDuplexMediaSelected",
	1501: "interpreterMemoryIncrease", 1502: "interpreterMemoryDecrease", 1503: "interpreterCartridgeAdded",
	1504: "interpreterCartridgeDeleted", 1505: "interpreterResourceUnavailable", 1506: "interpreterResourceAdded",
	1507: "interpreterResourceDeleted", 1509: "interpreterComplexPageEncountered", 1801: "alertRemovalOfBinaryChangeEntry",
}

// hrPrinterDetectedErrorState bit names (RFC 2790 §hrPrinterDetectedErrorState), bit 0 = MSB of octet 1.
var detectedErrorBits = []string{
	"lowPaper", "noPaper", "lowToner", "noToner", "doorOpen", "jammed", "offline", "serviceRequested",
	"inputTrayMissing", "outputTrayMissing", "markerSupplyMissing", "outputNearFull", "outputFull",
	"inputTrayEmpty", "overduePreventMaint",
}
