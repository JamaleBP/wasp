package datatypes

import (
	"bytes"
	"errors"
	"github.com/iotaledger/goshimmer/dapps/valuetransfers/packages/balance"
	"github.com/iotaledger/wasp/packages/kv"
	"github.com/iotaledger/wasp/packages/util"
)

type Map struct {
	kv         kv.KVStore
	name       string
	cachedsize uint32
}

type MustMap struct {
	m Map
}

const (
	mapSizeKeyCode = byte(0)
	mapElemKeyCode = byte(1)
)

func NewMap(kv kv.KVStore, name string) (*Map, error) {
	ret := &Map{
		kv:   kv,
		name: name,
	}
	var err error
	ret.cachedsize, err = ret.len()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func NewMustMap(m *Map) *MustMap {
	return &MustMap{*m}
}

func (m *Map) getSizeKey() kv.Key {
	var buf bytes.Buffer
	buf.Write([]byte(m.name))
	buf.WriteByte(mapSizeKeyCode)
	return kv.Key(buf.Bytes())
}

func (m *Map) getElemKey(key []byte) kv.Key {
	var buf bytes.Buffer
	buf.Write([]byte(m.name))
	buf.WriteByte(mapElemKeyCode)
	buf.Write(key)
	return kv.Key(buf.Bytes())
}

func (m *Map) setSize(size uint32) {
	if size == 0 {
		m.kv.Del(m.getSizeKey())
		return
	}
	m.cachedsize = size
	m.kv.Set(m.getSizeKey(), util.Uint32To4Bytes(size))
}

func (d *Map) GetAt(key []byte) ([]byte, error) {
	ret, err := d.kv.Get(d.getElemKey(key))
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (d *MustMap) GetAt(key []byte) []byte {
	ret, err := d.m.GetAt(key)
	if err != nil {
		panic(err)
	}
	return ret
}

func (d *Map) SetAt(key []byte, value []byte) error {
	if d.Len() == 0 {
		d.setSize(1)
	} else {
		ok, err := d.HasAt(key)
		if err != nil {
			return err
		}
		if !ok {
			d.setSize(d.Len() + 1)
		}
	}
	d.kv.Set(d.getElemKey(key), value)
	return nil
}

func (d *MustMap) SetAt(key []byte, value []byte) {
	_ = d.m.SetAt(key, value)
}

func (d *Map) DelAt(key []byte) error {
	ok, err := d.HasAt(key)
	if err != nil {
		return err
	}
	if ok {
		d.setSize(d.Len() - 1)
	}
	d.kv.Del(d.getElemKey(key))
	return nil
}

func (d *MustMap) DelAt(key []byte) {
	_ = d.m.DelAt(key)
}

func (d *Map) HasAt(key []byte) (bool, error) {
	return d.kv.Has(d.getElemKey(key))
}

func (d *MustMap) HasAt(key []byte) bool {
	ret, err := d.m.HasAt(key)
	if err != nil {
		panic(err)
	}
	return ret
}

func (d *Map) Len() uint32 {
	return d.cachedsize
}

func (d *MustMap) Len() uint32 {
	return d.m.cachedsize
}

func (d *Map) len() (uint32, error) {
	v, err := d.kv.Get(d.getSizeKey())
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, nil
	}
	if len(v) != 4 {
		return 0, errors.New("corrupted data")
	}
	return util.MustUint32From4Bytes(v), nil
}

func (d *Map) Erase() {
	// TODO needs DelPrefix method in KVStore
	panic("implement me")
}

// Iterate non-deterministic
func (d *Map) Iterate(f func(elemKey []byte, value []byte) bool) error {
	prefix := d.getElemKey(nil)
	return d.kv.Iterate(prefix, func(key kv.Key, value []byte) bool {
		return f([]byte(key)[len(prefix):], value)
	})
}

// Iterate non-deterministic
func (d *MustMap) Iterate(f func(elemKey []byte, value []byte) bool) {
	err := d.m.Iterate(f)
	if err != nil {
		panic(err)
	}
}

func (d *MustMap) IterateBalances(f func(color balance.Color, bal int64) bool) {
	d.Iterate(func(elemKey []byte, value []byte) bool {
		col, _, err := balance.ColorFromBytes(elemKey)
		if err != nil {
			panic(err)
		}
		bal := int64(util.MustUint64From8Bytes(value))
		return f(col, bal)
	})
}
