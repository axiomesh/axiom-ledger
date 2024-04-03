package precheck

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/core"
)

var precheckErrPrefix = errors.New("verify tx err")

var (
	errTxSign                       = errors.New("tx signature verify failed")
	errTo                           = errors.New("tx from and to address is same")
	errGasPriceTooLow               = errors.New("gas price too low")
	errFeeCapVeryHigh               = core.ErrFeeCapVeryHigh
	errTipVeryHigh                  = core.ErrTipVeryHigh
	errTipAboveFeeCap               = core.ErrTipAboveFeeCap
	errFeeCapTooLow                 = core.ErrFeeCapTooLow
	errMaxInitCodeSizeExceeded      = core.ErrMaxInitCodeSizeExceeded
	errInsufficientFunds            = core.ErrInsufficientFunds
	errIntrinsicGas                 = core.ErrIntrinsicGas
	errInsufficientFundsForTransfer = core.ErrInsufficientFundsForTransfer
)

var errorTypes = map[error]string{
	errTxSign:                       errTxSign.Error(),
	errTo:                           errTo.Error(),
	errGasPriceTooLow:               errGasPriceTooLow.Error(),
	errFeeCapVeryHigh:               errFeeCapVeryHigh.Error(),
	errTipVeryHigh:                  errTipVeryHigh.Error(),
	errTipAboveFeeCap:               errTipAboveFeeCap.Error(),
	errFeeCapTooLow:                 errFeeCapTooLow.Error(),
	errMaxInitCodeSizeExceeded:      errMaxInitCodeSizeExceeded.Error(),
	errInsufficientFunds:            errInsufficientFunds.Error(),
	errIntrinsicGas:                 errIntrinsicGas.Error(),
	errInsufficientFundsForTransfer: core.ErrInsufficientFundsForTransfer.Error(),
}

const (
	responseType_precheck = iota
	responseType_txPool
)

func convertErrorType(err error) (string, bool) {
	if err == nil {
		return "", true
	}

	for e, msg := range errorTypes {
		if errors.Is(err, e) {
			return msg, false
		}
	}

	return err.Error(), false
}

func wrapError(err error) error {
	return fmt.Errorf("%w:%w", precheckErrPrefix, err)
}
