package streamlistpack

import (
	"strconv"
)

type MasterEntry struct {
	count      int
	deleted    int
	fieldCount int
	fields     []string
}

// pushR entry -> 1 + N + 1 (tag + entry + backlen)
func (s *StreamListpack) PushMaster(data []string) {
	// push count and deleted
	s.lp.PushR("0") // count
	s.lp.PushR("0") // deleted

	fieldCount := len(data) / 2          // data len % 2 == 0 -> always
	s.lp.PushR(strconv.Itoa(fieldCount)) // field count

	for i := 0; i < len(data); i = i + 2 {
		s.lp.PushR(data[i])
	}
	s.lp.PushR("0") // terminator
}

func (s *StreamListpack) GetMaster() (*MasterEntry, error) {
	m := &MasterEntry{}

	e := s.lp.Read(0, 3)          // count, delete, fieldcount
	co, err := strconv.Atoi(e[0]) // count
	if err != nil {
		return nil, err
	}
	de, err := strconv.Atoi(e[1]) // delete
	if err != nil {
		return nil, err
	}
	fc, err := strconv.Atoi(e[2]) // field count
	if err != nil {
		return nil, err
	}

	m.count = co
	m.deleted = de
	m.fieldCount = fc
	m.fields = s.lp.Read(3, fc)

	return m, nil
}

func (s *StreamListpack) UpdateMasterCount(u int) error {
	co, err := strconv.Atoi(s.lp.Read(0, 1)[0]) // count
	if err != nil {
		return err
	}

	s.lp.PopL()                      // pop previous count
	s.lp.PushL(strconv.Itoa(co + u)) // push new count

	return nil
}

func (s *StreamListpack) Push(id []byte, data []string) {

}
