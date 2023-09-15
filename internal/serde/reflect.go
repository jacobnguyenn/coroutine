package serde

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"unsafe"
)

// A type is serialized as follow:
//
// - No type (t is nil) => varint(0)
// - Any type but array => varint(1-MaxInt)
// - Array type [X]T    => varint(-1) varint(X) [serialize T]
//
// This is so that we can represent slices as pointers to arrays, with a size
// not known at compile time (so precise array type hasn't been registered.
func serializeType(s *Serializer, t reflect.Type) {
	if t == nil {
		serializeVarint(s, 0)
		return
	}

	if t.Kind() != reflect.Array {
		serializeVarint(s, int(Types.idOf(t)))
		return
	}

	serializeVarint(s, -1)
	serializeVarint(s, t.Len())
	serializeType(s, t.Elem())
}

func deserializeType(d *Deserializer) reflect.Type {
	n := deserializeVarint(d)
	if n == 0 {
		return nil
	}

	if n > 0 {
		return Types.typeOf(sID(n))
	}

	if n != -1 {
		panic(fmt.Errorf("unknown type first int: %d", n))
	}

	l := deserializeVarint(d)
	et := deserializeType(d)
	return reflect.ArrayOf(l, et)
}

func SerializeAny(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	if serde, ok := Types.serdeOf(t); ok {
		serde.ser(s, p)
		return
	}

	switch t.Kind() {
	case reflect.Invalid:
		panic(fmt.Errorf("can't serialize reflect.Invalid"))
	case reflect.Bool:
		SerializeBool(s, *(*bool)(p))
	case reflect.Int:
		SerializeInt(s, *(*int)(p))
	case reflect.Int64:
		SerializeInt64(s, *(*int64)(p))
	case reflect.Int32:
		SerializeInt32(s, *(*int32)(p))
	case reflect.Int16:
		SerializeInt16(s, *(*int16)(p))
	case reflect.Int8:
		SerializeInt8(s, *(*int8)(p))
	case reflect.Uint:
		SerializeUint(s, *(*uint)(p))
	case reflect.Uint64:
		SerializeUint64(s, *(*uint64)(p))
	case reflect.Uint32:
		SerializeUint32(s, *(*uint32)(p))
	case reflect.Uint16:
		SerializeUint16(s, *(*uint16)(p))
	case reflect.Uint8:
		SerializeUint8(s, *(*uint8)(p))
	case reflect.Float64:
		SerializeFloat64(s, *(*float64)(p))
	case reflect.Float32:
		SerializeFloat32(s, *(*float32)(p))
	case reflect.Complex64:
		SerializeComplex64(s, *(*complex64)(p))
	case reflect.Complex128:
		SerializeComplex128(s, *(*complex128)(p))
	case reflect.String:
		SerializeString(s, (*string)(p))

	case reflect.Array:
		serializeArray(s, t, p)
	case reflect.Interface:
		serializeInterface(s, t, p)
	case reflect.Map:
		serializeMap(s, t, p)
	case reflect.Pointer:
		serializePointer(s, t, p)
	case reflect.Slice:
		serializeSlice(s, t, p)
	case reflect.Struct:
		serializeStruct(s, t, p)
	// Chan
	// Func
	// UnsafePointer
	default:
		panic(fmt.Errorf("reflection cannot serialize type %s", t))
	}
}

func DeserializeAny(d *Deserializer, t reflect.Type, p unsafe.Pointer) {
	if serde, ok := Types.serdeOf(t); ok {
		serde.des(d, p)
		return
	}

	switch t.Kind() {
	case reflect.Invalid:
		panic(fmt.Errorf("can't deserialize reflect.Invalid"))
	case reflect.Bool:
		DeserializeBool(d, (*bool)(p))
	case reflect.Int:
		DeserializeInt(d, (*int)(p))
	case reflect.Int64:
		DeserializeInt64(d, (*int64)(p))
	case reflect.Int32:
		DeserializeInt32(d, (*int32)(p))
	case reflect.Int16:
		DeserializeInt16(d, (*int16)(p))
	case reflect.Int8:
		DeserializeInt8(d, (*int8)(p))
	case reflect.Uint:
		DeserializeUint(d, (*uint)(p))
	case reflect.Uint64:
		DeserializeUint64(d, (*uint64)(p))
	case reflect.Uint32:
		DeserializeUint32(d, (*uint32)(p))
	case reflect.Uint16:
		DeserializeUint16(d, (*uint16)(p))
	case reflect.Uint8:
		DeserializeUint8(d, (*uint8)(p))
	case reflect.Float64:
		DeserializeFloat64(d, (*float64)(p))
	case reflect.Float32:
		DeserializeFloat32(d, (*float32)(p))
	case reflect.Complex64:
		DeserializeComplex64(d, (*complex64)(p))
	case reflect.Complex128:
		DeserializeComplex128(d, (*complex128)(p))
	case reflect.String:
		DeserializeString(d, (*string)(p))
	case reflect.Interface:
		deserializeInterface(d, t, p)
	case reflect.Pointer:
		deserializePointer(d, t, p)
	case reflect.Array:
		deserializeArray(d, t, p)
	case reflect.Slice:
		deserializeSlice(d, t, p)
	case reflect.Map:
		deserializeMap(d, t, p)
	case reflect.Struct:
		deserializeStruct(d, t, p)
	default:
		panic(fmt.Errorf("reflection cannot deserialize type %s", t))
	}
}

func serializePointedAt(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	//	fmt.Printf("Serialize pointed at: %d (%s)\n", p, t)
	// If this is a nil pointer, write it as such.
	if p == nil {
		//		fmt.Printf("\t=>NIL\n")
		serializeVarint(s, 0)
		return
	}

	id, new := s.assignPointerID(p)
	serializeVarint(s, int(id))
	//	fmt.Printf("\t=>Assigned ID %d\n", id)
	if !new {
		//		fmt.Printf("\t=>Already seen\n")
		// This exact pointer has already been serialized. Write its ID
		// and move on.
		return
	}
	//	fmt.Printf("\t=>New pointer\n")

	// Now, this is pointer that is seen for the first time.

	// Check the region of this pointer.
	r := s.regions.regionOf(p)

	// If this pointer does not belong to any region or is the container of
	// the region, write a negative offset to flag it is on its own, and
	// write its data.
	if !r.valid() || (r.offset(p) == 0 && t == r.typ) {
		//		fmt.Printf("\t=>Is container (region %t)\n", r.Valid())
		serializeVarint(s, -1)
		SerializeAny(s, t, p)
		return
	}

	// The pointer points into a memory region.
	offset := r.offset(p)
	serializeVarint(s, offset)

	//	fmt.Printf("\t=>Offset in container: %d\n", offset)

	// Write the type of the container.
	serializeType(s, r.typ)
	//	fmt.Printf("\t=>Container at: %d (%s)\n", r.Pointer(), r.Type())

	// Serialize the parent.
	serializePointedAt(s, r.typ, r.start)
}

func deserializePointedAt(d *Deserializer, t reflect.Type) reflect.Value {
	// This function is a bit different than the other deserialize* ones
	// because it deserializes into an unknown location. As a result,
	// instead of taking an unsafe.Pointer as an input, it returns a
	// reflect.Value that contains a *T (where T is given by the argument
	// t).

	//	fmt.Printf("Deserialize pointed at: %s\n", t)

	ptr, id := d.readPtr()
	//	fmt.Printf("\t=> ptr=%d, id=%d\n", ptr, id)
	if ptr != nil || id == 0 { // pointer already seen or nil
		//		fmt.Printf("\t=>Returning existing data\n")
		return reflect.NewAt(t, ptr)
	}

	offset := deserializeVarint(d)
	//	fmt.Printf("\t=>Read offset %d\n", offset)

	// Negative offset means this is either a container or a standalone
	// value.
	if offset < 0 {
		e := reflect.New(t)
		ep := e.UnsafePointer()
		d.store(id, ep)
		//		fmt.Printf("\t=>Negative offset: container %d\n", ep)
		DeserializeAny(d, t, ep)
		return e
	}

	// This pointer points into a container. Deserialize that one first,
	// then return the pointer itself with an offset.
	ct := deserializeType(d)

	//	fmt.Printf("\t=>Container type: %s\n", ct)

	// cp is a pointer to the container
	cp := deserializePointedAt(d, ct)

	// Create the pointer with an offset into the container.
	ep := unsafe.Add(cp.UnsafePointer(), offset)
	r := reflect.NewAt(t, ep)
	d.store(id, ep)
	//	fmt.Printf("\t=>Returning id=%d ep=%d\n", id, ep)
	return r
}

func serializeMap(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	size := 0
	r := reflect.NewAt(t, p).Elem()
	if r.IsNil() {
		size = -1
	} else {
		size = r.Len()
	}
	serializeVarint(s, size)

	// TODO: allocs
	iter := r.MapRange()
	k := reflect.New(t.Key()).Elem()
	v := reflect.New(t.Elem()).Elem()
	for iter.Next() {
		k.Set(iter.Key())
		v.Set(iter.Value())
		SerializeAny(s, t.Key(), k.Addr().UnsafePointer())
		SerializeAny(s, t.Elem(), v.Addr().UnsafePointer())
	}
}

func deserializeMap(d *Deserializer, t reflect.Type, p unsafe.Pointer) {
	n := deserializeVarint(d)
	if n < 0 { // nil map
		return
	}
	nv := reflect.MakeMapWithSize(t, n)
	r := reflect.NewAt(t, p)
	r.Elem().Set(nv)
	for i := 0; i < n; i++ {
		k := reflect.New(t.Key())
		DeserializeAny(d, t.Key(), k.UnsafePointer())
		v := reflect.New(t.Elem())
		DeserializeAny(d, t.Elem(), v.UnsafePointer())
		r.Elem().SetMapIndex(k.Elem(), v.Elem())
	}
}

func serializeSlice(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	r := reflect.NewAt(t, p).Elem()

	serializeVarint(s, r.Len())
	serializeVarint(s, r.Cap())

	at := reflect.ArrayOf(r.Cap(), t.Elem())
	ap := r.UnsafePointer()

	serializePointedAt(s, at, ap)
}

func deserializeSlice(d *Deserializer, t reflect.Type, p unsafe.Pointer) {
	l := deserializeVarint(d)
	c := deserializeVarint(d)

	at := reflect.ArrayOf(c, t.Elem())
	ar := deserializePointedAt(d, at)

	if ar.IsNil() {
		return
	}

	s := (*slice)(p)
	s.data = ar.UnsafePointer()
	s.cap = c
	s.len = l
}

func serializeArray(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	n := t.Len()
	te := t.Elem()
	ts := int(te.Size())
	for i := 0; i < n; i++ {
		pe := unsafe.Add(p, ts*i)
		SerializeAny(s, te, pe)
	}
}

func deserializeArray(d *Deserializer, t reflect.Type, p unsafe.Pointer) {
	size := int(t.Elem().Size())
	te := t.Elem()
	for i := 0; i < t.Len(); i++ {
		pe := unsafe.Add(p, size*i)
		DeserializeAny(d, te, pe)
	}
}

func serializePointer(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	r := reflect.NewAt(t, p).Elem()
	x := r.UnsafePointer()
	serializePointedAt(s, t.Elem(), x)
}

func deserializePointer(d *Deserializer, t reflect.Type, p unsafe.Pointer) {
	ep := deserializePointedAt(d, t.Elem())
	r := reflect.NewAt(t, p)
	r.Elem().Set(ep)
}

func serializeStruct(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	n := t.NumField()
	for i := 0; i < n; i++ {
		ft := t.Field(i)
		fp := unsafe.Add(p, ft.Offset)
		SerializeAny(s, ft.Type, fp)
	}
}

func deserializeStruct(d *Deserializer, t reflect.Type, p unsafe.Pointer) {
	n := t.NumField()
	for i := 0; i < n; i++ {
		ft := t.Field(i)
		fp := unsafe.Add(p, ft.Offset)
		DeserializeAny(d, ft.Type, fp)
	}
}

func serializeInterface(s *Serializer, t reflect.Type, p unsafe.Pointer) {
	i := (*iface)(p)

	if i.typ == nil {
		serializeType(s, nil)
		return
	}

	x := *(*interface{})(p)
	et := reflect.TypeOf(x)
	serializeType(s, et)

	eptr := i.ptr
	if inlined(et) {
		xp := i.ptr
		eptr = unsafe.Pointer(&xp)
		// noescape?
	}

	serializePointedAt(s, et, eptr)
}

func deserializeInterface(d *Deserializer, t reflect.Type, p unsafe.Pointer) {
	// Deserialize the type
	et := deserializeType(d)
	if et == nil {
		return
	}

	// Deserialize the pointer
	ep := deserializePointedAt(d, et)

	// Store the result in the interface
	r := reflect.NewAt(t, p)
	r.Elem().Set(ep.Elem())
}

func SerializeString(s *Serializer, x *string) {
	// Serialize string as a size and a pointer to an array of bytes.

	l := len(*x)
	serializeVarint(s, l)

	if l == 0 {
		return
	}

	at := reflect.ArrayOf(l, byteT)
	ap := unsafe.Pointer(unsafe.StringData(*x))

	serializePointedAt(s, at, ap)
}

func DeserializeString(d *Deserializer, x *string) {
	l := deserializeVarint(d)

	if l == 0 {
		return
	}

	at := reflect.ArrayOf(l, byteT)
	ar := deserializePointedAt(d, at)

	*x = unsafe.String((*byte)(ar.UnsafePointer()), l)
}

func SerializeBool(s *Serializer, x bool) {
	c := byte(0)
	if x {
		c = 1
	}
	s.b = append(s.b, c)
}

func DeserializeBool(d *Deserializer, x *bool) {
	*x = d.b[0] == 1
	d.b = d.b[1:]
}

func SerializeInt(s *Serializer, x int) {
	SerializeInt64(s, int64(x))
}

func DeserializeInt(d *Deserializer, x *int) {
	*x = int(binary.LittleEndian.Uint64(d.b[:8]))
	d.b = d.b[8:]
}

func SerializeInt64(s *Serializer, x int64) {
	s.b = binary.LittleEndian.AppendUint64(s.b, uint64(x))
}

func DeserializeInt64(d *Deserializer, x *int64) {
	*x = int64(binary.LittleEndian.Uint64(d.b[:8]))
	d.b = d.b[8:]
}

func SerializeInt32(s *Serializer, x int32) {
	s.b = binary.LittleEndian.AppendUint32(s.b, uint32(x))
}

func DeserializeInt32(d *Deserializer, x *int32) {
	*x = int32(binary.LittleEndian.Uint32(d.b[:4]))
	d.b = d.b[4:]
}

func SerializeInt16(s *Serializer, x int16) {
	s.b = binary.LittleEndian.AppendUint16(s.b, uint16(x))
}

func DeserializeInt16(d *Deserializer, x *int16) {
	*x = int16(binary.LittleEndian.Uint16(d.b[:2]))
	d.b = d.b[2:]
}

func SerializeInt8(s *Serializer, x int8) {
	s.b = append(s.b, byte(x))
}

func DeserializeInt8(d *Deserializer, x *int8) {
	*x = int8(d.b[0])
	d.b = d.b[1:]
}

func SerializeUint(s *Serializer, x uint) {
	SerializeUint64(s, uint64(x))
}

func DeserializeUint(d *Deserializer, x *uint) {
	*x = uint(binary.LittleEndian.Uint64(d.b[:8]))
	d.b = d.b[8:]
}

func SerializeUint64(s *Serializer, x uint64) {
	s.b = binary.LittleEndian.AppendUint64(s.b, x)
}

func DeserializeUint64(d *Deserializer, x *uint64) {
	*x = uint64(binary.LittleEndian.Uint64(d.b[:8]))
	d.b = d.b[8:]
}

func SerializeUint32(s *Serializer, x uint32) {
	s.b = binary.LittleEndian.AppendUint32(s.b, x)
}

func DeserializeUint32(d *Deserializer, x *uint32) {
	*x = uint32(binary.LittleEndian.Uint32(d.b[:4]))
	d.b = d.b[4:]
}

func SerializeUint16(s *Serializer, x uint16) {
	s.b = binary.LittleEndian.AppendUint16(s.b, x)
}

func DeserializeUint16(d *Deserializer, x *uint16) {
	*x = uint16(binary.LittleEndian.Uint16(d.b[:2]))
	d.b = d.b[2:]
}

func SerializeUint8(s *Serializer, x uint8) {
	s.b = append(s.b, byte(x))
}

func DeserializeUint8(d *Deserializer, x *uint8) {
	*x = uint8(d.b[0])
	d.b = d.b[1:]
}

func SerializeFloat32(s *Serializer, x float32) {
	SerializeUint32(s, math.Float32bits(x))
}

func DeserializeFloat32(d *Deserializer, x *float32) {
	DeserializeUint32(d, (*uint32)(unsafe.Pointer(x)))
}

func SerializeFloat64(s *Serializer, x float64) {
	SerializeUint64(s, math.Float64bits(x))
}

func DeserializeFloat64(d *Deserializer, x *float64) {
	DeserializeUint64(d, (*uint64)(unsafe.Pointer(x)))
}

func SerializeComplex64(s *Serializer, x complex64) {
	SerializeFloat32(s, real(x))
	SerializeFloat32(s, imag(x))
}

func DeserializeComplex64(d *Deserializer, x *complex64) {
	type complex64 struct {
		real float32
		img  float32
	}
	p := (*complex64)(unsafe.Pointer(x))
	DeserializeFloat32(d, &p.real)
	DeserializeFloat32(d, &p.img)
}

func SerializeComplex128(s *Serializer, x complex128) {
	SerializeFloat64(s, real(x))
	SerializeFloat64(s, imag(x))
}

func DeserializeComplex128(d *Deserializer, x *complex128) {
	type complex128 struct {
		real float64
		img  float64
	}
	p := (*complex128)(unsafe.Pointer(x))
	DeserializeFloat64(d, &p.real)
	DeserializeFloat64(d, &p.img)
}

var (
	byteT = reflect.TypeOf(byte(0))
)
