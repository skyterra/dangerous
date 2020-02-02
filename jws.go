package dangerous

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"time"
)

var (
	Jws_algorithms = map[string]interface{}{
		"HS256": HMACAlgorithm{DigestMethod: sha256.New},
		"HS384": HMACAlgorithm{DigestMethod: sha512.New384},
		"HS512": HMACAlgorithm{DigestMethod: sha512.New},
		"none":  NoneAlgorithm{}}

	Default_algorithm = "HS512"

	Default_serializer = Json{}

	DEFAULT_EXPIRES_IN int64 = 3600
)

type JSONWebSignatureSerializer struct {
	serializer    Serializer
	Secret        string
	Salt          string
	Serializer    JsonAPI
	Signer        Signer
	Signerkwargs  map[string]interface{}
	AlgorithmName string
	Algorithm     Signature
	Expires_in    int64
}

func (self *JSONWebSignatureSerializer) SetDefault() {
	if self.AlgorithmName == "" {
		self.AlgorithmName = Default_algorithm
	}
	if self.Expires_in == 0 {
		self.Expires_in = DEFAULT_EXPIRES_IN
	}
	self.Algorithm = Jws_algorithms[self.AlgorithmName].(Signature)
	self.Serializer = Json{}
	ser := &Serializer{
		Secret:       self.Secret,
		Salt:         self.Salt,
		SerializerOP: self.Serializer,
		Signer:       self.Signer,
		Signerkwargs: self.Signerkwargs}
	ser.SetDefault()
	self.serializer = (*ser)
}

func (self JSONWebSignatureSerializer) LoadPayload(payload []byte) (interface{}, interface{}, error) {
	null := []byte("")
	sep := []byte(".")
	if !bytes.Contains(payload, sep) {
		return null, null, fmt.Errorf("BadPayload: No '.' found in value, %s", payload)
	}
	v := bytes.SplitN(payload, []byte("."), 2)

	base64d_header, base64d_payload := v[0], v[1]

	json_header, err := B64decode(base64d_header)
	if err != nil {
		return json_header, null, fmt.Errorf("Could not base64 decode the header because of an exception")
	}
	json_payload, err := B64decode(base64d_payload)
	if err != nil {
		return null, json_payload, fmt.Errorf("Could not base64 decode the payload because of an exception")
	}
	header, err := self.serializer.LoadPayload(json_header)
	if err != nil {
		return header, null, fmt.Errorf("Could not unserialize header because it was malformed")
	}
	_, ok := header.(map[string]interface{})
	if !ok {
		return header, null, fmt.Errorf("Header payload is not a JSON object")
	}
	payloadr, err := self.serializer.LoadPayload(json_payload)
	return header, payloadr, err
}

func (self JSONWebSignatureSerializer) DumpPayload(header, obj interface{}) []byte {
	h, _ := self.Serializer.(JsonAPI).Dump(header)
	base64d_header := B64encode([]byte(h))
	p, _ := self.Serializer.(JsonAPI).Dump(obj)
	base64d_payload := B64encode([]byte(p))
	sep := WantBytes(".")
	result, _ := Concentrate(WantBytes(base64d_header), sep, WantBytes(base64d_payload))
	return result
}

func (self JSONWebSignatureSerializer) MakeSigner() Signer {
	key_derivation := ""
	if self.Salt == "" {
		key_derivation = "none"
	}
	SIGNER := &Signer{
		Secret:        self.Secret,
		Salt:          self.Salt,
		Sep:           ".",
		KeyDerivation: key_derivation,
		Algorithm:     self.Algorithm,
	}
	SIGNER.SetDefault()
	return (*SIGNER)

}

func (self JSONWebSignatureSerializer) MakeHeader(header_fields map[string]interface{}) map[string]interface{} {
	header_fields["alg"] = self.AlgorithmName
	return header_fields

}

func (self JSONWebSignatureSerializer) Dumps(obj interface{}, args ...interface{}) []byte {
	(&self).SetDefault()
	header_fields := map[string]interface{}{}
	if len(args) == 1 {
		header_fields, _ = args[0].(map[string]interface{})
	}
	header := self.MakeHeader(header_fields)
	signer := self.MakeSigner()
	return signer.Sign(string(self.DumpPayload(header, obj)))
}

func (self JSONWebSignatureSerializer) Loads(s string) (interface{}, interface{}, error) {
	(&self).SetDefault()
	signer := self.MakeSigner()
	b, err := signer.UnSign(s)
	if err != nil {
		return nil, nil, err
	}
	h, payload, err := self.LoadPayload(b)
	header, _ := h.(map[string]interface{})
	if header["alg"].(string) != self.AlgorithmName {
		err = fmt.Errorf(`BadHeader: Algorithm mismatch, header:%b, payload=%b`, header, payload)
	}
	return header, payload, err
}

func (self JSONWebSignatureSerializer) TimedMakeHeader(header_fields map[string]interface{}) map[string]interface{} {
	header := self.MakeHeader(header_fields)
	iat := self.now()
	exp := iat + self.Expires_in
	header["iat"] = iat
	header["exp"] = exp
	return header
}

func (self JSONWebSignatureSerializer) TimedDumps(obj interface{}, args ...interface{}) []byte {
	(&self).SetDefault()
	header_fields := map[string]interface{}{}
	if len(args) == 1 {
		header_fields, _ = args[0].(map[string]interface{})
		fmt.Println("get")
	}
	header := self.TimedMakeHeader(header_fields)
	return self.Dumps(obj, header)
}

func (self JSONWebSignatureSerializer) TimedLoads(s string) (map[string]interface{}, interface{}, error) {
	(&self).SetDefault()
	header, payload, err := self.Loads(s)
	if err != nil {
		return header, payload, err
	}
	headers := header.(map[string]interface{})
	if ok := headers["exp"]; ok == nil {
		return headers, payload, fmt.Errorf("BadSignature-Missing expiry date.")
	}

	int_date_error := fmt.Errorf(`BadHeader-Expiry date is not an IntDate, payload:%v`, payload)

	expfloat, ok := headers["exp"].(float64)
	if !ok {
		return headers, payload, int_date_error
	}
	exp := int64(expfloat)
	if exp < 0 {
		return headers, payload, int_date_error
	}

	if exp < self.now() {
		err := fmt.Errorf(`Signature expired, expired at %s.`, self.Get_issue_date(exp))
		return headers, payload, err
	}
	return headers, payload, nil

}

func (self JSONWebSignatureSerializer) now() int64 {
	return time.Now().UTC().Unix()
}

func (self JSONWebSignatureSerializer) Get_issue_date(t int64) string {
	return fmt.Sprintf("%s", time.Unix(t, 0).UTC())
}
