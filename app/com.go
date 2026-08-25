package main

import (
	"errors"
)

// #D1 -> using []string as arg value for now. If it does not work out, go for any in the future

type Com struct {
	Base string
	Args map[string][]string
	// for all optional args
	// some commands support optional args
	Extras map[string][]string
}

func GetCom(base string, args []string) (*Com, error) {
	com := &Com{
		Base:   base,
		Args:   make(map[string][]string),
		Extras: make(map[string][]string),
	}

	var err error

	switch base {
	case "ping":
		com.ValidatePing(args)
	case "echo":
		err = com.ValidateEcho(args)
	case "get":
		err = com.ValidateGet(args)
	case "set":
		err = com.ValidateSet(args)
	case "rpush":
		err = com.ValidateRPush(args)
	case "lrange":
		err = com.ValidateLRange(args)
	case "lpush":
		err = com.ValidateLPush(args)
	case "llen":
		err = com.ValidateLLen(args)
	case "lpop":
		err = com.ValidateLPop(args)
	case "blpop":
		err = com.ValidateBLPop(args)
	case "type":
		err = com.ValidateType(args)
	case "xadd":
		err = com.ValidateXAdd(args)
	}

	return com, err
}

func (com *Com) HandleCom() []byte {
	switch com.Base {
	case "ping":
		return com.ping()
	case "echo":
		return com.echo()
	case "get":
		return com.get()
	case "set":
		return com.set()
	case "rpush":
		return com.rpush()
	case "lrange":
		return com.lrange()
	case "lpush":
		return com.lpush()
	case "llen":
		return com.llen()
	case "lpop":
		return com.lpop()
	case "blpop":
		return com.blpop()
	case "type":
		return com.handleType()
	case "xadd":
		return com.xadd()

	default:
		return nil
	}
}

func (com *Com) ValidatePing(args []string) {} // eat five star do nothing

func (com *Com) ValidateEcho(args []string) error {
	if len(args) >= 1 {
		com.Args["echothis"] = []string{args[0]}
	} else {
		return errors.New("Echo expects atleast one argument")
	}

	return nil
}

func (com *Com) ValidateGet(args []string) error {
	if len(args) >= 1 {
		com.Args["key"] = []string{args[0]}
	} else {
		return errors.New("Get expects key to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateSet(args []string) error {
	// key
	if len(args) >= 1 {
		com.Args["key"] = []string{args[0]}
	} else {
		return errors.New("Set expects key to be passed as an argument")
	}
	// value
	if len(args) >= 2 {
		com.Args["value"] = []string{args[1]}
	} else {
		return errors.New("Set expects value to be passed as an argument")
	}

	// extras
	// (@iAmAdheil) -> set up validation for non-compulsary args in the future
	if len(args) >= 3 {
		com.Extras["expiryType"] = []string{args[2]}
	}
	if len(args) >= 4 {
		com.Extras["duration"] = []string{args[3]}
	}

	return nil
}

func (com *Com) ValidateRPush(args []string) error {
	// key
	if len(args) >= 1 {
		com.Args["listkey"] = []string{args[0]}
	} else {
		return errors.New("RPush expects key to be passed as an argument")
	}
	// value
	if len(args) >= 2 {
		com.Args["values"] = args[1:]
	} else {
		return errors.New("RPush expects atleast 1 value to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateLRange(args []string) error {
	if len(args) >= 1 {
		com.Args["listkey"] = []string{args[0]}
	} else {
		return errors.New("LRange expects key to be passed as an argument")
	}
	if len(args) >= 2 {
		com.Args["left"] = []string{args[1]}
	} else {
		return errors.New("LRange expects left index to be passed as an argument")
	}
	if len(args) >= 3 {
		com.Args["right"] = []string{args[2]}
	} else {
		return errors.New("LRange expects right index to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateLPush(args []string) error {
	// key
	if len(args) >= 1 {
		com.Args["listkey"] = []string{args[0]}
	} else {
		return errors.New("LPush expects key to be passed as an argument")
	}
	// value
	if len(args) >= 2 {
		com.Args["values"] = args[1:]
	} else {
		return errors.New("LPush expects atleast 1 value to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateLLen(args []string) error {
	// key
	if len(args) >= 1 {
		com.Args["listkey"] = []string{args[0]}
	} else {
		return errors.New("LLen expects key to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateLPop(args []string) error {
	if len(args) >= 1 {
		com.Args["listkey"] = []string{args[0]}
	} else {
		return errors.New("LPop expects key to be passed as an argument")
	}
	if len(args) >= 2 {
		com.Args["count"] = []string{args[1]}
	} else {
		com.Args["count"] = []string{"1"}
	}

	return nil
}

func (com *Com) ValidateBLPop(args []string) error {
	if len(args) >= 1 {
		com.Args["listkey"] = []string{args[0]}
	} else {
		return errors.New("BLPop expects key to be passed as an argument")
	}
	if len(args) >= 2 {
		com.Args["timeout"] = []string{args[1]}
	} else {
		return errors.New("BLPop expects timeout to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateType(args []string) error {
	// key
	if len(args) >= 1 {
		com.Args["key"] = []string{args[0]}
	} else {
		return errors.New("Type expects key to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateXAdd(args []string) error {
	if len(args) >= 1 {
		com.Args["streamkey"] = []string{args[0]}
	} else {
		return errors.New("XAdd expects stream key to be passed as an argument")
	}
	if len(args) >= 2 {
		com.Args["id"] = []string{args[1]}
	} else {
		return errors.New("XAdd expects id to be passed as an argument")
	}

	args = args[2:]
	com.Args["data"] = args

	return nil
}
