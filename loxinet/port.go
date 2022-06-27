/*
 * Copyright (c) 2022 NetLOX Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package loxinet

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    tk "loxilb/loxilib"
    "net"
    "strings"
)

const (
    PORT_BASE_ERR = iota - 1000
    PORT_EXISTS_ERR
    PORT_NOTEXIST_ERR
    PORT_NOMASTER_ERR
    PORT_COUNTER_ERR
    PORT_MAP_ERR
    PORT_ZONE_ERR
    PORT_NOREALDEV_ERR
)

const (
    MAX_BOND_IFS = 8
    MAX_PHY_IFS  = 128
    MAX_IFS      = 512
)

const (
    PORT_REAL     = 0x1
    PORT_BONDSIF  = 0x2
    PORT_BOND     = 0x4
    PORT_VLANSIF  = 0x8
    PORT_VLANBR   = 0x10
    PORT_VXLANSIF = 0x20
    PORT_VXLANBR  = 0x40
    PORT_WG       = 0x80
)

const (
    REAL_PORT_VB = 3800
    BOND_VB      = 4000
)

type PortEvent uint

const (
    PORT_EV_DOWN PortEvent = 1 << iota
    PORT_EV_LOWER_DOWN
    PORT_EV_DELETE
)

type PortEventIntf interface {
    PortNotifier(name string, osId int, evType PortEvent)
}

type PortStatsInfo struct {
    RxBytes   uint64
    TxBytes   uint64
    RxPackets uint64
    TxPackets uint64
    RxError   uint64
    TxError   uint64
}

type PortHwInfo struct {
    MacAddr [6]byte
    Link    bool
    State   bool
    Mtu     int
    Master  string
    Real    string
    TunId   uint32
}

type PortLayer3Info struct {
    Routed     bool
    Ipv4_addrs []string
    Ipv6_addrs []string
}

type PortSwInfo struct {
    OsId       int
    PortType   int
    PortActive bool
    PortReal   *Port
    PortOvl    *Port
}

type PortLayer2Info struct {
    IsPvid bool
    Vid    int
}

type Port struct {
    Name   string
    PortNo int
    Zone   string
    SInfo  PortSwInfo
    HInfo  PortHwInfo
    Stats  PortStatsInfo
    L3     PortLayer3Info
    L2     PortLayer2Info
    Sync   DpStatusT
}

type PortsH struct {
    portImap   []*Port
    portSmap   map[string]*Port
    portOmap   map[int]*Port
    portNotifs []PortEventIntf
    portHwMark *tk.Counter
    bondHwMark *tk.Counter
}

func PortInit() *PortsH {
    var nllp = new(PortsH)
    nllp.portImap = make([]*Port, MAX_IFS)
    nllp.portSmap = make(map[string]*Port)
    nllp.portOmap = make(map[int]*Port)
    nllp.portHwMark = tk.NewCounter(1, MAX_IFS)
    nllp.bondHwMark = tk.NewCounter(1, MAX_BOND_IFS)
    return nllp
}

func (P *PortsH) PortGetSlaves(master string) (int, []*Port) {
    var slaves []*Port

    for _, p := range P.portSmap {
        if p.HInfo.Master == master {
            slaves = append(slaves, p)
        }
    }

    return 0, slaves
}

func (P *PortsH) PortHasTunSlaves(master string, ptype int) (bool, []*Port) {
    var slaves []*Port

    for _, p := range P.portSmap {
        if p.HInfo.Master == master &&
           p.SInfo.PortType & ptype == ptype {
            slaves = append(slaves, p)
        }
    }

    if len(slaves) > 0 {
        return true, slaves
    }
    return false, nil
}

func (P *PortsH) PortAdd(name string, osid int, ptype int, zone string,
    hwi PortHwInfo, l2i PortLayer2Info) (int, error) {

    if _, err := mh.zn.ZonePortIsValid(name, zone); err != nil {
        return PORT_ZONE_ERR, errors.New("no such zone")
    }

    zn, _ := mh.zn.Zonefind(zone)
    if zn == nil {
        return PORT_ZONE_ERR, errors.New("no such zone")
    }

    if P.portSmap[name] != nil {
        p := P.portSmap[name]
        if p.SInfo.PortType == PORT_REAL {
            if ptype == PORT_VLANSIF &&
                l2i.IsPvid == true {
                p.HInfo.Master = hwi.Master
                p.SInfo.PortType |= ptype
                if p.L2 != l2i {
                    p.DP(DP_REMOVE)

                    p.L2 = l2i
                    p.DP(DP_CREATE)
                    tk.LogIt(tk.LOG_DEBUG, "[PORT ADD] Port %v vlan info updated\n", name)
                    return 0, nil
                }
            }
            if ptype == PORT_BONDSIF {
                master := P.portSmap[hwi.Master]
                if master == nil {
                    return PORT_NOMASTER_ERR, errors.New("no such master")
                }
                p.DP(DP_REMOVE)

                p.SInfo.PortType |= ptype
                p.HInfo.Master = hwi.Master
                p.L2.IsPvid = true
                p.L2.Vid = master.PortNo + BOND_VB

                p.DP(DP_CREATE)
                return 0, nil
            }

        } else if p.SInfo.PortType == PORT_BOND {
            if ptype == PORT_VLANSIF &&
                l2i.IsPvid == true {
                if p.L2 != l2i {

                    p.DP(DP_REMOVE)

                    p.L2 = l2i

                    p.SInfo.PortType |= ptype
                    p.DP(DP_CREATE)
                    return 0, nil
                }
            }
        }
        if p.SInfo.PortType == PORT_VXLANBR {
            if ptype == PORT_VLANSIF &&
                l2i.IsPvid == true {
                p.HInfo.Master = hwi.Master
                p.SInfo.PortType |= ptype
                p.DP(DP_REMOVE)
                p.L2 = l2i
                p.DP(DP_CREATE)
                tk.LogIt(tk.LOG_DEBUG, "[PORT ADD] Port %v vlan info updated\n", name)
                return 0, nil
            }
        }
        return PORT_EXISTS_ERR, errors.New("port exists")
    }

    var rid int
    var err error

    if ptype == PORT_BOND {
        rid, err = P.bondHwMark.GetCounter()
    } else {
        rid, err = P.portHwMark.GetCounter()
    }
    if err != nil {
        return PORT_COUNTER_ERR, err
    }

    var rp *Port = nil
    if hwi.Real != "" {
        rp = P.portSmap[hwi.Real]
        if rp == nil {
            return PORT_NOREALDEV_ERR, errors.New("no such real port")
        }
    } else if ptype == PORT_VXLANBR {
        return PORT_NOREALDEV_ERR, errors.New("need real-dev info")
    }

    p := new(Port)
    p.Name = name
    p.Zone = zone
    p.HInfo = hwi
    p.PortNo = rid
    p.SInfo.PortActive = true
    p.SInfo.OsId = osid
    p.SInfo.PortType = ptype
    p.SInfo.PortReal = rp

    vMac := [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

    switch ptype {
    case PORT_REAL:
        p.L2.IsPvid = true
        p.L2.Vid = rid + REAL_PORT_VB

        /* We create an vlan BD to keep things in sync */
        vstr := fmt.Sprintf("vlan%d", p.L2.Vid)
        zn.Vlans.VlanAdd(p.L2.Vid, vstr, zone, -1,
            PortHwInfo{vMac, true, true, 9000, "", "", 0})
    case PORT_BOND:
        p.L2.IsPvid = true
        p.L2.Vid = rid + BOND_VB

        /* We create an vlan BD to keep things in sync */
        vstr := fmt.Sprintf("vlan%d", p.L2.Vid)
        zn.Vlans.VlanAdd(p.L2.Vid, vstr, zone, -1,
            PortHwInfo{vMac, true, true, 9000, "", "", 0})
    case PORT_VXLANBR:
        if p.SInfo.PortReal != nil {
            p.SInfo.PortReal.SInfo.PortOvl = p
        }
        p.L2.IsPvid = true
        p.L2.Vid = int(p.HInfo.TunId)
    default:
        tk.LogIt(tk.LOG_DEBUG, "%s: isPVid %v\n", p.Name, p.L2.IsPvid)
        p.L2 = l2i
    }

    //fmt.Println(p)

    P.portSmap[name] = p
    P.portImap[rid] = p
    P.portOmap[osid] = p

    mh.zn.ZonePortAdd(name, zone)
    tk.LogIt(tk.LOG_DEBUG, "[PORT ADD] Port %s %d\n", p.Name, p.PortNo)
    p.DP(DP_CREATE)

    return 0, nil
}

func (P *PortsH) PortDel(name string, ptype int) (int, error) {
    if P.portSmap[name] == nil {
        return PORT_NOTEXIST_ERR, errors.New("no such port")
    }

    p := P.portSmap[name]

    // If phy port was access vlan, it is converted to normal phy port
    // If it has a trunk vlan association, we will have a subinterface
    if (p.SInfo.PortType&(PORT_REAL|PORT_VLANSIF) == (PORT_REAL|PORT_VLANSIF)) &&
        ptype == PORT_VLANSIF {
        p.DP(DP_REMOVE)

        p.SInfo.PortType = p.SInfo.PortType & ^PORT_VLANSIF
        p.HInfo.Master = ""
        p.L2.IsPvid = true
        p.L2.Vid = p.PortNo + REAL_PORT_VB
        p.DP(DP_CREATE)
        return 0, nil
    }

    if (p.SInfo.PortType&(PORT_VXLANBR|PORT_VLANSIF) == (PORT_VXLANBR|PORT_VLANSIF)) &&
        ptype == PORT_VXLANBR {
        p.DP(DP_REMOVE)

        p.SInfo.PortType = p.SInfo.PortType & ^PORT_VLANSIF
        p.HInfo.Master = ""
        p.L2.IsPvid = true
        p.L2.Vid = int(p.HInfo.TunId)
        p.DP(DP_CREATE)
        return 0, nil
    }

    if (p.SInfo.PortType&(PORT_BOND|PORT_VLANSIF) == (PORT_BOND | PORT_VLANSIF)) &&
        ptype == PORT_VLANSIF {
        p.DP(DP_REMOVE)
        p.SInfo.PortType = p.SInfo.PortType & ^PORT_VLANSIF
        p.L2.IsPvid = true
        p.L2.Vid = p.PortNo + BOND_VB
        p.DP(DP_CREATE)
        return 0, nil
    }

    if (p.SInfo.PortType&(PORT_REAL|PORT_BONDSIF) == (PORT_REAL | PORT_BONDSIF)) &&
        ptype == PORT_BONDSIF {
        p.DP(DP_REMOVE)
        p.SInfo.PortType = p.SInfo.PortType & ^PORT_BONDSIF
        p.HInfo.Master = ""
        p.L2.IsPvid = true
        p.L2.Vid = p.PortNo + REAL_PORT_VB
        p.DP(DP_CREATE)
        return 0, nil
    }

    rid := P.portSmap[name].PortNo

    if P.portImap[rid] == nil {
        return PORT_MAP_ERR, errors.New("no such port in imap")
    }

    if P.portOmap[P.portSmap[name].SInfo.OsId] == nil {
        return PORT_MAP_ERR, errors.New("no such port in omap")
    }

    p.DP(DP_REMOVE)

    switch p.SInfo.PortType {
    case PORT_VXLANBR:
        if p.SInfo.PortReal != nil {
            p.SInfo.PortReal.SInfo.PortOvl = nil
        }
    case PORT_REAL:
    case PORT_BOND:
        zone := mh.zn.GetPortZone(p.Name)
        if zone != nil {
            zone.Vlans.VlanDelete(p.L2.Vid)
        }
        break
    }

    p.SInfo.PortReal = nil
    p.SInfo.PortActive = false
    mh.zn.ZonePortDelete(name)

    // TODO - Need to clear layer2 and layer3 information
    delete(P.portOmap, p.SInfo.OsId)
    delete(P.portSmap, name)
    P.portImap[rid] = nil

    return 0, nil
}

func (P *PortsH) PortUpdate() {
}

func (P *PortsH) Ports2Json(w io.Writer) error {

    for _, e := range P.portSmap {
        js, err := json.Marshal(e)
        if err != nil {
            return err
        }
        _, err = w.Write(js)
    }

    return nil
}

func port2String(e *Port, it IterIntf) {
    var s string
    var pStr string
    //var portStr string;
    if e.HInfo.State {
        pStr = "UP"
    } else {
        pStr = "DOWN"
    }

    if e.HInfo.Link {
        pStr += ",RUNNING"
    }

    s += fmt.Sprintf("%-10s: <%s> mtu %d %s\n",
        e.Name, pStr, e.HInfo.Mtu, e.Zone)

    pStr = ""
    if e.SInfo.PortType&PORT_REAL == PORT_REAL {
        pStr += "phy,"
    }
    if e.SInfo.PortType&PORT_VLANSIF == PORT_VLANSIF {
        pStr += "vlan-sif,"
    }
    if e.SInfo.PortType&PORT_VLANBR == PORT_VLANBR {
        pStr += "vlan,"
    }
    if e.SInfo.PortType&PORT_BONDSIF == PORT_BONDSIF {
        pStr += "bond-sif,"
    }
    if e.SInfo.PortType&PORT_BONDSIF == PORT_BOND {
        pStr += "bond,"
    }
    if e.SInfo.PortType&PORT_VXLANSIF == PORT_VXLANSIF {
        pStr += "vxlan-sif,"
    }
    if e.SInfo.PortType&PORT_VXLANBR == PORT_VXLANBR {
        pStr += "vxlan"
        if e.SInfo.PortReal != nil {
            pStr += fmt.Sprintf("(%s)", e.SInfo.PortReal.Name)
        }
    }

    nStr := strings.TrimSuffix(pStr, ",")
    s += fmt.Sprintf("%-10s  ether %02x:%02x:%02x:%02x:%02x:%02x  %s\n",
        "", e.HInfo.MacAddr[0], e.HInfo.MacAddr[1], e.HInfo.MacAddr[2],
        e.HInfo.MacAddr[3], e.HInfo.MacAddr[4], e.HInfo.MacAddr[5], nStr)
    it.NodeWalker(s)
}

func (P *PortsH) Ports2String(it IterIntf) error {
    for _, e := range P.portSmap {
        port2String(e, it)
    }
    return nil
}

func (P *PortsH) PortFindByName(name string) (p *Port) {
    p, _ = P.portSmap[name]
    return p
}

func (P *PortsH) PortFindByOSId(osId int) (p *Port) {
    p, _ = P.portOmap[osId]
    return p
}

func (P *PortsH) PortL2AddrMatch(name string, mp *Port) bool {
    p := P.PortFindByName(name)
    if p != nil {
        if p.HInfo.MacAddr == mp.HInfo.MacAddr {
            return true
        }
    }
    return false
}

func (P *PortsH) PortNotifierRegister(notifier PortEventIntf) {
    P.portNotifs = append(P.portNotifs, notifier)
}

func (P *PortsH) PortTicker() {
    var ev PortEvent
    var portMod = false
    for _, port := range P.portSmap {
        portMod = false

        // TODO - This is not very efficient since internally
        // it will get all OS interfaces each time
        osIntf, err := net.InterfaceByName(port.Name)
        if err == nil {
            // Delete Port - TODO
            continue
        }

        // TODO - check link status also ??
        // Currently golang's net package does not extract it
        if !port.HInfo.State {
            if osIntf.Flags&net.FlagUp != 0 {
                port.HInfo.State = true
                ev = 0
                portMod = true
            }
        } else {
            if osIntf.Flags&net.FlagUp == 0 {
                port.HInfo.State = false
                ev = PORT_EV_DOWN
                portMod = true
            }
        }

        if portMod {
            for _, notif := range P.portNotifs {
                notif.PortNotifier(port.Name, port.SInfo.OsId, ev)
            }
        }

    }
}

func (P *PortsH) PortDestructAll() {
    var realDevs []*Port
    var bSlaves []*Port
    var bridges []*Port
    var bondSlaves []*Port
    var bonds []*Port
    var tunSlaves []*Port
    var tunnels []*Port

    for _, p := range P.portSmap {

        if p.SInfo.PortType&PORT_REAL == PORT_REAL {
            realDevs = append(realDevs, p)
        }
        if p.SInfo.PortType&PORT_VLANSIF == PORT_VLANSIF {
            bSlaves = append(bSlaves, p)
        }
        if p.SInfo.PortType&PORT_VLANBR == PORT_VLANBR {
            bridges = append(bridges, p)
        }
        if p.SInfo.PortType&PORT_BONDSIF == PORT_BONDSIF {
            bondSlaves = append(bondSlaves, p)
        }
        if p.SInfo.PortType&PORT_BONDSIF == PORT_BOND {
            bonds = append(bonds, p)
        }
        if p.SInfo.PortType&PORT_VXLANSIF == PORT_VXLANSIF {
            tunSlaves = append(tunSlaves, p)
        }
        if p.SInfo.PortType&PORT_VXLANBR == PORT_VXLANBR {
            tunnels = append(tunnels, p)
        }
    }

    for _, p := range tunSlaves {
        P.PortDel(p.Name, PORT_VXLANSIF)
    }

    for _, p := range bSlaves {
        P.PortDel(p.Name, PORT_VLANSIF)
    }

    for _, p := range bondSlaves {
        P.PortDel(p.Name, PORT_BONDSIF)
    }

    for _, p := range bonds {
        P.PortDel(p.Name, PORT_BOND)
    }

    for _, p := range bridges {
        P.PortDel(p.Name, PORT_VLANBR)
    }

    for _, p := range tunnels {
        P.PortDel(p.Name, PORT_VXLANBR)
    }

    for _, p := range realDevs {
        P.PortDel(p.Name, PORT_REAL)
    }
}

func (p *Port) DP(work DpWorkT) int {

    zn, zoneNum := mh.zn.Zonefind(p.Zone)
    if zoneNum < 0 {
        return -1
    }

    // When a vxlan interface is created
    if p.SInfo.PortType == PORT_VXLANBR {
        // Do nothing
        return 0
    }

    // When a vxlan interface becomes slave of a bridge
    if p.SInfo.PortType & (PORT_VXLANBR | PORT_VLANSIF) == (PORT_VXLANBR | PORT_VLANSIF) {
        rmWq := new(RouterMacDpWorkQ)
        rmWq.Work = work
        rmWq.Status = nil

        if p.SInfo.PortReal == nil {
            return -1
        }

        up := p.SInfo.PortReal

        for i := 0; i < 6; i++ {
            rmWq.l2Addr[i] = uint8(up.HInfo.MacAddr[i])
        }
        rmWq.PortNum = up.PortNo
        rmWq.TunId = p.HInfo.TunId
        rmWq.TunType = DP_TUN_VXLAN
        rmWq.BD = p.L2.Vid

        mh.dp.ToDpCh <- rmWq

        return 0
    }

    // When bond subinterface e.g bond1.100 is created
    if p.SInfo.PortType == PORT_VLANSIF && p.SInfo.PortReal != nil &&
        p.SInfo.PortReal.SInfo.PortType&PORT_BOND == PORT_BOND {

        pWq := new(PortDpWorkQ)

        pWq.Work = work
        pWq.PortNum = p.SInfo.PortReal.PortNo
        pWq.OsPortNum = p.SInfo.PortReal.SInfo.OsId
        pWq.IngVlan = p.L2.Vid
        pWq.SetBD = p.L2.Vid
        pWq.SetZoneNum = zoneNum
        mh.dp.ToDpCh <- pWq

        return 0
    }

    // When bond becomes a vlan-port e.g bond1 ==> vlan200
    if p.SInfo.PortType&(PORT_BOND|PORT_VLANSIF) == (PORT_BOND | PORT_VLANSIF) {
        _, slaves := zn.Ports.PortGetSlaves(p.Name)
        for _, sp := range slaves {
            pWq := new(PortDpWorkQ)
            pWq.Work = work
            pWq.OsPortNum = sp.SInfo.OsId
            pWq.PortNum = sp.PortNo
            pWq.IngVlan = 0
            pWq.SetBD = p.L2.Vid
            pWq.SetZoneNum = zoneNum

            mh.dp.ToDpCh <- pWq
        }
        return 0
    }

    if (p.SInfo.PortType&PORT_REAL != PORT_REAL) &&
        (p.SInfo.PortReal == nil || p.SInfo.PortReal.SInfo.PortType&PORT_REAL != PORT_REAL) {
        return 0
    }

    pWq := new(PortDpWorkQ)

    pWq.Work = work

    if p.SInfo.PortReal != nil {
        pWq.OsPortNum = p.SInfo.PortReal.SInfo.OsId
        pWq.PortNum = p.SInfo.PortReal.PortNo
    } else {
        pWq.OsPortNum = p.SInfo.OsId
        pWq.PortNum = p.PortNo
    }

    if p.L2.IsPvid {
        pWq.IngVlan = 0
    } else {
        pWq.IngVlan = p.L2.Vid
    }

    pWq.SetBD = p.L2.Vid
    _, pWq.SetZoneNum = mh.zn.Zonefind(p.Zone)

    if pWq.SetZoneNum < 0 {
        return -1
    }

    if (work == DP_CREATE || work == DP_REMOVE) &&
        p.SInfo.PortType&PORT_REAL == PORT_REAL ||
        p.SInfo.PortType&PORT_BOND == PORT_BOND {

        pWq.LoadEbpf = p.Name
    } else {
        pWq.LoadEbpf = ""
    }

    // TODO - Need to unload eBPF when port properties change

    mh.dp.ToDpCh <- pWq

    return 0
}