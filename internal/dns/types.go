package dns

type Record struct {
	SubDomain string
	Type      string
	Value     string
	Priority  *uint64
	Remark    string
	TTL       *uint64
}

type Plan struct {
	Domain     string
	RecordLine string
	Records    []Record
}
