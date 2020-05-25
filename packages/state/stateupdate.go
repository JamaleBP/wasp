package state

import (
	"github.com/iotaledger/wasp/packages/sctransaction"
	"github.com/iotaledger/wasp/packages/util"
	"github.com/iotaledger/wasp/packages/variables"
	"io"
)

type stateUpdate struct {
	batchIndex uint16
	requestId  sctransaction.RequestId
	vars       variables.Variables
}

func NewStateUpdate(reqid *sctransaction.RequestId) StateUpdate {
	var req sctransaction.RequestId
	if reqid != nil {
		req = *reqid
	}
	return &stateUpdate{
		requestId: req,
		vars:      variables.New(nil),
	}
}

func NewStateUpdateRead(r io.Reader) (StateUpdate, error) {
	ret := NewStateUpdate(nil).(*stateUpdate)
	return ret, ret.Read(r)
}

// StateUpdate

func (su *stateUpdate) RequestId() *sctransaction.RequestId {
	return &su.requestId
}

func (su *stateUpdate) BatchIndex() uint16 {
	return su.batchIndex
}

func (su *stateUpdate) SetBatchIndex(batchIndex uint16) {
	su.batchIndex = batchIndex
}

func (su *stateUpdate) Variables() variables.Variables {
	return su.vars
}

func (su *stateUpdate) Write(w io.Writer) error {
	if err := util.WriteUint16(w, su.batchIndex); err != nil {
		return err
	}
	if _, err := w.Write(su.requestId[:]); err != nil {
		return err
	}
	if err := su.vars.Write(w); err != nil {
		return err
	}
	return nil
}

func (su *stateUpdate) Read(r io.Reader) error {
	if err := util.ReadUint16(r, &su.batchIndex); err != nil {
		return err
	}
	if _, err := r.Read(su.requestId[:]); err != nil {
		return err
	}
	if err := su.vars.Read(r); err != nil {
		return err
	}
	return nil
}