package general_tests

import (
	"net/rpc"
	"net/rpc/jsonrpc"
	"path"
	"testing"
	"time"

	"github.com/cgrates/cgrates/apier/v1"
	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/engine"
	"github.com/cgrates/cgrates/utils"
)

var tpCfgPath string
var tpCfg *config.CGRConfig
var tpRPC *rpc.Client
var tpLoadInst engine.LoadInstance // Share load information between tests

func TestTpInitCfg(t *testing.T) {
	if !*testIntegration {
		return
	}
	tpCfgPath = path.Join(*dataDir, "conf", "samples", "tutlocal")
	// Init config first
	var err error
	tpCfg, err = config.NewCGRConfigFromFolder(tpCfgPath)
	if err != nil {
		t.Error(err)
	}
	tpCfg.DataFolderPath = *dataDir // Share DataFolderPath through config towards StoreDb for Flush()
	config.SetCgrConfig(tpCfg)
}

// Remove data in both rating and accounting db
func TestTpResetDataDb(t *testing.T) {
	if !*testIntegration {
		return
	}
	if err := engine.InitDataDb(tpCfg); err != nil {
		t.Fatal(err)
	}
}

// Wipe out the cdr database
func TestTpResetStorDb(t *testing.T) {
	if !*testIntegration {
		return
	}
	if err := engine.InitStorDb(tpCfg); err != nil {
		t.Fatal(err)
	}
}

// Start CGR Engine
func TestTpStartEngine(t *testing.T) {
	if !*testIntegration {
		return
	}
	if _, err := engine.StopStartEngine(tpCfgPath, *waitRater); err != nil {
		t.Fatal(err)
	}
}

// Connect rpc client to rater
func TestTpRpcConn(t *testing.T) {
	if !*testIntegration {
		return
	}
	var err error
	tpRPC, err = jsonrpc.Dial("tcp", tpCfg.RPCJSONListen) // We connect over JSON so we can also troubleshoot if needed
	if err != nil {
		t.Fatal(err)
	}
}

// Load the tariff plan, creating accounts and their balances
func TestTpLoadTariffPlanFromFolder(t *testing.T) {
	if !*testIntegration {
		return
	}
	attrs := &utils.AttrLoadTpFromFolder{FolderPath: path.Join(*dataDir, "tariffplans", "testtp")}
	if err := tpRPC.Call("ApierV2.LoadTariffPlanFromFolder", attrs, &tpLoadInst); err != nil {
		t.Error(err)
	} else if tpLoadInst.LoadId == "" {
		t.Error("Empty loadId received, loadInstance: ", tpLoadInst)
	}
	time.Sleep(time.Duration(*waitRater) * time.Millisecond) // Give time for scheduler to execute topups
}

func TestTpBalanceCounter(t *testing.T) {
	if !*testIntegration {
		return
	}
	tStart := time.Date(2016, 3, 31, 0, 0, 0, 0, time.UTC)
	cd := engine.CallDescriptor{
		Direction:     "*out",
		Category:      "call",
		Tenant:        "cgrates.org",
		Subject:       "1001",
		Destination:   "+49",
		DurationIndex: 0,
		TimeStart:     tStart,
		TimeEnd:       tStart.Add(time.Duration(20) * time.Second),
	}
	var cc engine.CallCost
	if err := tpRPC.Call("Responder.Debit", cd, &cc); err != nil {
		t.Error("Got error on Responder.GetCost: ", err.Error())
	} else if cc.GetDuration() != 20*time.Second {
		t.Errorf("Calling Responder.MaxDebit got callcost: %v", cc.GetDuration())
	}
	var acnt *engine.Account
	attrs := &utils.AttrGetAccount{Tenant: "cgrates.org", Account: "1001"}
	if err := tpRPC.Call("ApierV2.GetAccount", attrs, &acnt); err != nil {
		t.Error("Got error on ApierV2.GetAccount: ", err.Error())
	} else if acnt.UnitCounters[utils.MONETARY][1].Counters[0].Value != 20.0 {
		t.Errorf("Calling ApierV2.GetBalance received: %s", utils.ToIJSON(acnt))
	}
}

func TestTpActionTriggers(t *testing.T) {
	if !*testIntegration {
		return
	}
	var atrs engine.ActionTriggers
	if err := tpRPC.Call("ApierV1.GetActionTriggers", v1.AttrGetActionTriggers{GroupIDs: []string{}}, &atrs); err != nil {
		t.Error("Got error on ApierV1.GetActionTriggers: ", err.Error())
	} else if len(atrs) != 9 {
		t.Errorf("Calling v1.GetActionTriggers got: %v", atrs)
	}
	var reply string
	if err := tpRPC.Call("ApierV1.SetActionTrigger", v1.AttrSetActionTrigger{
		GroupID:   "TestATR",
		UniqueID:  "Unique atr id",
		BalanceID: utils.StringPointer("BID1"),
	}, &reply); err != nil {
		t.Error("Got error on ApierV1.SetActionTrigger: ", err.Error())
	} else if reply != utils.OK {
		t.Errorf("Calling v1.SetActionTrigger got: %v", reply)
	}

	if err := tpRPC.Call("ApierV1.GetActionTriggers", v1.AttrGetActionTriggers{GroupIDs: []string{}}, &atrs); err != nil {
		t.Error("Got error on ApierV1.GetActionTriggers: ", err.Error())
	} else if len(atrs) != 10 {
		t.Errorf("Calling v1.GetActionTriggers got: %v", atrs)
	}
	if err := tpRPC.Call("ApierV1.GetActionTriggers", v1.AttrGetActionTriggers{GroupIDs: []string{"TestATR"}}, &atrs); err != nil {
		t.Error("Got error on ApierV1.GetActionTriggers: ", err.Error())
	} else if len(atrs) != 1 {
		t.Errorf("Calling v1.GetActionTriggers got: %v", atrs)
	}
	if atrs[0].ID != "TestATR" ||
		atrs[0].UniqueID != "Unique atr id" ||
		*atrs[0].Balance.ID != "BID1" {
		t.Error("Wrong action trigger set: ", utils.ToIJSON(atrs[0]))
	}
}

func TestTpZeroCost(t *testing.T) {
	if !*testIntegration {
		return
	}
	var acnt *engine.Account
	attrs := &utils.AttrGetAccount{Tenant: "cgrates.org", Account: "1012"}
	if err := tpRPC.Call("ApierV2.GetAccount", attrs, &acnt); err != nil {
		t.Error("Got error on ApierV2.GetAccount: ", err.Error())
	}
	balanceValueBefore := acnt.BalanceMap[utils.MONETARY][0].Value
	tStart := time.Date(2016, 3, 31, 0, 0, 0, 0, time.UTC)
	cd := engine.CallDescriptor{
		Direction:     "*out",
		Category:      "call",
		Tenant:        "cgrates.org",
		Subject:       "free",
		Account:       "1012",
		Destination:   "+49",
		DurationIndex: 0,
		TimeStart:     tStart,
		TimeEnd:       tStart.Add(time.Duration(20) * time.Second),
	}
	var cc engine.CallCost
	if err := tpRPC.Call("Responder.Debit", cd, &cc); err != nil {
		t.Error("Got error on Responder.Debit: ", err.Error())
	} else if cc.GetDuration() != 20*time.Second {
		t.Errorf("Calling Responder.MaxDebit got callcost: %v", utils.ToIJSON(cc))
	}
	if err := tpRPC.Call("ApierV2.GetAccount", attrs, &acnt); err != nil {
		t.Error("Got error on ApierV2.GetAccount: ", err.Error())
	} else if acnt.BalanceMap[utils.MONETARY][0].Value != balanceValueBefore {
		t.Errorf("Calling ApierV2.GetAccount received: %s", utils.ToIJSON(acnt))
	}
}

func TestTpZeroNegativeCost(t *testing.T) {
	if !*testIntegration {
		return
	}
	tStart := time.Date(2016, 3, 31, 0, 0, 0, 0, time.UTC)
	cd := engine.CallDescriptor{
		Direction:     "*out",
		Category:      "call",
		Tenant:        "cgrates.org",
		Subject:       "free",
		Account:       "1013",
		Destination:   "+4915",
		DurationIndex: 0,
		TimeStart:     tStart,
		TimeEnd:       tStart.Add(time.Duration(20) * time.Second),
	}
	var cc engine.CallCost
	if err := tpRPC.Call("Responder.Debit", cd, &cc); err != nil {
		t.Error("Got error on Responder.GetCost: ", err.Error())
	} else if cc.GetDuration() != 20*time.Second {
		t.Errorf("Calling Responder.MaxDebit got callcost: %v", utils.ToIJSON(cc))
	}
	var acnt *engine.Account
	attrs := &utils.AttrGetAccount{Tenant: "cgrates.org", Account: "1013"}
	if err := tpRPC.Call("ApierV2.GetAccount", attrs, &acnt); err != nil {
		t.Error("Got error on ApierV2.GetAccount: ", err.Error())
	} else if acnt.BalanceMap[utils.VOICE][0].Value != 100.0 {
		t.Errorf("Calling ApierV2.GetAccount received: %s", utils.ToIJSON(acnt))
	}
}
