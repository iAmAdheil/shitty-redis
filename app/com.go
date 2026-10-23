package main

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/codecrafters-io/redis-starter-go/app/server"
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
	case "xrange":
		err = com.ValidateXRange(args)
	case "xread":
		err = com.ValidateXRead(args)
	case "incr":
		err = com.ValidateIncr(args)
	case "multi":
		err = com.ValidateMulti(args)
	case "exec":
		err = com.ValidateExec(args)
	case "discard":
		err = com.ValidateDiscard(args)
	case "watch":
		err = com.ValidateWatch(args)
	case "unwatch":
		err = com.ValidateUnwatch(args)
	case "info":
		err = com.ValidateInfo(args)
	}

	return com, err
}

func (com *Com) HandleCom(server *server.Server, state *State) ([]byte, error) {
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
	case "xrange":
		return com.xrange()
	case "xread":
		return com.xread()
	case "incr":
		return com.incr()
	case "multi":
		return com.multi(state)
	case "exec":
		return com.exec(server, state)
	case "discard":
		return com.discard(state)
	case "watch":
		return com.watch(state)
	case "unwatch":
		return com.unwatch(state)
	case "info":
		return com.info(server)

	default:
		return nil, nil
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
		com.Args["key"] = []string{args[0]}
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
		com.Args["key"] = []string{args[0]}
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
		com.Args["key"] = []string{args[0]}
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
		com.Args["key"] = []string{args[0]}
	} else {
		return errors.New("LLen expects key to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateLPop(args []string) error {
	if len(args) >= 1 {
		com.Args["key"] = []string{args[0]}
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
		com.Args["key"] = []string{args[0]}
	} else {
		return errors.New("BLPop expects key to be passed as an argument")
	}

	// default value
	com.Args["timeout"] = []string{strconv.FormatInt(0, 10)}
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
	var key string
	if len(args) >= 1 {
		key = args[0]
		com.Args["key"] = []string{key}
	} else {
		return errors.New("XAdd expects stream key to be passed as an argument")
	}

	if len(args) >= 2 {
		com.Args["id"] = []string{args[1]}
	} else {
		return errors.New("XAdd expects id to be passed as an argument")
	}

	if len(args) >= 3 {
		args = args[2:]
		com.Args["data"] = args
	} else {
		// panic prevention
		com.Args["data"] = []string{}
	}

	return nil
}

func (com *Com) ValidateXRange(args []string) error {
	if len(args) >= 1 {
		com.Args["key"] = []string{args[0]}
	} else {
		return errors.New("XRange expects stream key to be passed as an argument")
	}
	if len(args) >= 2 {
		com.Args["low"] = []string{args[1]}
	} else {
		return errors.New("XRange expects lower range to be passed as an argument")
	}
	if len(args) >= 3 {
		com.Args["high"] = []string{args[2]}
	} else {
		return errors.New("XRange expects upper range to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateXRead(args []string) error {
	target := "streams"

	var copy []string // check for 'streams' keyword -> case insensitive
	for _, arg := range args {
		copy = append(copy, strings.ToLower(arg))
	}

	idx := slices.Index(copy, target)
	if idx == -1 {
		return errors.New("Xread expects 'STREAMS' to be passed as a required argument")
	}

	nonReqs := args[:idx]
	reqs := args[idx+1:]

	// build req args
	if len(reqs)%2 != 0 {
		return errors.New("Invalid XRead arguments")
	}
	for i := 0; i < len(reqs); i++ {
		c := len(reqs) / 2
		com.Args["keys"] = reqs[:c]
		com.Args["ids"] = reqs[c:]
	}

	// build non-req args
	for i := 0; i < len(nonReqs); i++ {
		opt := nonReqs[i]
		i++
		val := nonReqs[i]

		com.Extras[strings.ToLower(opt)] = []string{val}
	}

	return nil
}

func (com *Com) ValidateIncr(args []string) error {
	if len(args) >= 1 {
		com.Args["key"] = []string{args[0]}
	} else {
		return errors.New("Incr expects a key to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateMulti(args []string) error {
	return nil
}

func (com *Com) ValidateExec(args []string) error {
	return nil
}

func (com *Com) ValidateDiscard(args []string) error {
	return nil
}

func (com *Com) ValidateWatch(args []string) error {
	if len(args) >= 1 {
		keys := []string{}
		for _, key := range args {
			keys = append(keys, key)
		}
		com.Args["keys"] = keys
	} else {
		return errors.New("Watch expects atleast one key to be passed as an argument")
	}

	return nil
}

func (com *Com) ValidateUnwatch(args []string) error {
	return nil
}

func (com *Com) ValidateInfo(args []string) error {
	if len(args) >= 1 {
		// optional
		com.Extras["replicate"] = []string{"_"}
	}

	return nil
}
