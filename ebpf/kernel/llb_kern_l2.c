/*
 *  llb_kern_L2.c: LoxiLB kernel eBPF L2 Processing Implementation
 *  Copyright (C) 2022,  NetLOX <www.netlox.io>
 * 
 * SPDX-License-Identifier: GPL-2.0
 */
static int __always_inline
dp_do_smac_lkup(void *ctx, struct xfi *F, void *fc)
{
  struct dp_smac_key key;
  struct dp_smac_tact *sma;

  memcpy(key.smac, F->l2m.dl_src, 6);
  key.bd = F->pm.bd;

  LL_DBG_PRINTK("[SMAC] -- Lookup\n");
  LL_DBG_PRINTK("[SMAC] %x:%x:%x\n",
                 key.smac[0], key.smac[1], key.smac[2]);
  LL_DBG_PRINTK("[SMAC] %x:%x:%x\n",
                 key.smac[3], key.smac[4], key.smac[5]);
  LL_DBG_PRINTK("[SMAC] BD%d\n", key.bd);

  F->pm.table_id = LL_DP_SMAC_MAP;

  sma = bpf_map_lookup_elem(&smac_map, &key);
  if (!sma) {
    /* Default action */
    LLBS_PPLN_PASS(F);
    return 0;
  }

  LL_DBG_PRINTK("[SMAC] action %d\n", sma->ca.act_type);

  if (sma->ca.act_type == DP_SET_DROP) {
    LLBS_PPLN_DROP(F);
  } else if (sma->ca.act_type == DP_SET_TOCP) {
    LLBS_PPLN_TRAP(F);
  } else if (sma->ca.act_type == DP_SET_NOP) {
    /* Nothing to do */
    return 0;
  } else {
    LLBS_PPLN_DROP(F);
  }

  return 0;
}

static int __always_inline
dp_pipe_set_l22_tun_nh(void *ctx, struct xfi *F, struct dp_rt_nh_act *rnh)
{
  F->pm.nh_num = rnh->nh_num;

  /*
   * We do not set out_bd here. After NH lookup match is
   * found and packet tunnel insertion is done, BD is set accordingly
   */
  /*F->pm.bd = rnh->bd;*/
  F->tm.new_tunnel_id = rnh->tid;
  LL_DBG_PRINTK("[TMAC] new-vx nh %u\n", F->pm.nh_num);
  return 0;
}

static int __always_inline
dp_pipe_set_strip_vx_tun(void *ctx, struct xfi *F, struct dp_rt_nh_act *rnh)
{
  F->pm.phit &= ~LLB_DP_TMAC_HIT;
  F->pm.bd = rnh->bd;

  LL_DBG_PRINTK("[TMAC] strip-vx newbd %d \n", F->pm.bd);
  return dp_pop_outer_metadata(ctx, F);
}

static int __always_inline
__dp_do_tmac_lkup(void *ctx, struct xfi *F,
                  int tun_lkup, void *fa_)
{
  struct dp_tmac_key key;
  struct dp_tmac_tact *tma;
#ifdef HAVE_DP_FC
  struct dp_fc_tacts *fa = fa_;
#endif

  memcpy(key.mac, F->l2m.dl_dst, 6);
  key.pad  = 0;
  if (tun_lkup) {
    key.tunnel_id = F->tm.tunnel_id;
    key.tun_type = F->tm.tun_type;
  } else {
    key.tunnel_id = 0;
    key.tun_type  = 0;
  }

  LL_DBG_PRINTK("[TMAC] -- Lookup\n");
  LL_DBG_PRINTK("[TMAC] %x:%x:%x\n",
                 key.mac[0], key.mac[1], key.mac[2]);
  LL_DBG_PRINTK("[TMAC] %x:%x:%x\n",
                 key.mac[3], key.mac[4], key.mac[5]);
  LL_DBG_PRINTK("[TMAC] %x:%x\n", key.tunnel_id, key.tun_type);

  F->pm.table_id = LL_DP_TMAC_MAP;

  tma = bpf_map_lookup_elem(&tmac_map, &key);
  if (!tma) {
    /* No L3 lookup */
    return 0;
  }

  LL_DBG_PRINTK("[TMAC] action %d %d\n", tma->ca.act_type, tma->ca.cidx);
  if (tma->ca.cidx != 0) {
    dp_do_map_stats(ctx, F, LL_DP_TMAC_STATS_MAP, tma->ca.cidx);
  }

  if (tma->ca.act_type == DP_SET_DROP) {
    LLBS_PPLN_DROP(F);
  } else if (tma->ca.act_type == DP_SET_TOCP) {
    LLBS_PPLN_TRAP(F);
  } else if (tma->ca.act_type == DP_SET_RT_TUN_NH) {
#ifdef HAVE_DP_FC
    struct dp_fc_tact *ta = &fa->fcta[DP_SET_RT_TUN_NH];
    ta->ca.act_type = DP_SET_RT_TUN_NH;
    memcpy(&ta->nh_act,  &tma->rt_nh, sizeof(tma->rt_nh));
#endif
    return dp_pipe_set_l22_tun_nh(ctx, F, &tma->rt_nh);
  } else if (tma->ca.act_type == DP_SET_L3_EN) {
    F->pm.phit |= LLB_DP_TMAC_HIT;
  } else if (tma->ca.act_type == DP_SET_RM_VXLAN) {
#ifdef HAVE_DP_FC
    struct dp_fc_tact *ta = &fa->fcta[DP_SET_RM_VXLAN];
    ta->ca.act_type = DP_SET_RM_VXLAN;
    memcpy(&ta->nh_act,  &tma->rt_nh, sizeof(tma->rt_nh));
#endif
    return dp_pipe_set_strip_vx_tun(ctx, F, &tma->rt_nh);
  }

  return 0;
}

static int __always_inline
dp_do_tmac_lkup(void *ctx, struct xfi *F, void *fa)
{
  return __dp_do_tmac_lkup(ctx, F, 0, fa);
}

static int __always_inline
dp_do_tun_lkup(void *ctx, struct xfi *F, void *fa)
{
  if (F->tm.tunnel_id != 0) {
    return __dp_do_tmac_lkup(ctx, F, 1, fa);
  }
  return 0;
}

static int __always_inline
dp_set_egr_vlan(void *ctx, struct xfi *F,
                __u16 vlan, __u16 oport)
{
  LLBS_PPLN_RDR(F);
  F->pm.oport = oport;
  F->pm.bd = vlan;
  LL_DBG_PRINTK("[SETVLAN] OP %u V %u\n", oport, vlan);
  return 0;
}

static int __always_inline
dp_do_dmac_lkup(void *ctx, struct xfi *F, void *fa_)
{
  struct dp_dmac_key key;
  struct dp_dmac_tact *dma;
#ifdef HAVE_DP_FC
  struct dp_fc_tacts *fa = fa_;
#endif

  memcpy(key.dmac, F->pm.lkup_dmac, 6);
  key.bd = F->pm.bd;
  F->pm.table_id = LL_DP_DMAC_MAP;

  LL_DBG_PRINTK("[DMAC] -- Lookup \n");
  LL_DBG_PRINTK("[DMAC] %x:%x:%x\n",
                 key.dmac[0], key.dmac[1], key.dmac[2]);
  LL_DBG_PRINTK("[DMAC] %x:%x:%x\n", 
                 key.dmac[3], key.dmac[4], key.dmac[5]);
  LL_DBG_PRINTK("[DMAC] BD %d\n", key.bd);

  dma = bpf_map_lookup_elem(&dmac_map, &key);
  if (!dma) {
    /* No DMAC lookup */
    LL_DBG_PRINTK("[DMAC] not found\n");
    LLBS_PPLN_PASS(F);
    return 0;
  }

  LL_DBG_PRINTK("[DMAC] action %d pipe %d\n",
                 dma->ca.act_type, F->pm.pipe_act);

  if (dma->ca.act_type == DP_SET_DROP) {
    LLBS_PPLN_DROP(F);
  } else if (dma->ca.act_type == DP_SET_TOCP) {
    LLBS_PPLN_TRAP(F);
  } else if (dma->ca.act_type == DP_SET_RDR_PORT) {
    struct dp_rdr_act *ra = &dma->port_act;

    LLBS_PPLN_RDR(F);
    F->pm.oport = ra->oport;
    LL_DBG_PRINTK("[DMAC] oport %lu\n", F->pm.oport);
    return 0;
  } else if (dma->ca.act_type == DP_SET_ADD_L2VLAN || 
             dma->ca.act_type == DP_SET_RM_L2VLAN) {
    struct dp_l2vlan_act *va = &dma->vlan_act;
#ifdef HAVE_DP_FC
    struct dp_fc_tact *ta = &fa->fcta[
                          dma->ca.act_type == DP_SET_ADD_L2VLAN ?
                          DP_SET_ADD_L2VLAN : DP_SET_RM_L2VLAN];
    ta->ca.act_type = dma->ca.act_type;
    memcpy(&ta->l2ov,  va, sizeof(*va));
#endif
    return dp_set_egr_vlan(ctx, F, 
                    dma->ca.act_type == DP_SET_RM_L2VLAN ?
                    0 : va->vlan, va->oport);
  }

  return 0;
}

static int
dp_do_rt_l2_nh(void *ctx, struct xfi *F,
               struct dp_rt_l2nh_act *nl2)
{
  memcpy(F->l2m.dl_dst, nl2->dmac, 6);
  memcpy(F->l2m.dl_src, nl2->smac, 6);
  memcpy(F->pm.lkup_dmac, nl2->dmac, 6);
  F->pm.bd = nl2->bd;
 
  return nl2->rnh_num;
}

static int 
dp_do_rt_l2_vxlan_nh(void *ctx, struct xfi *F,
                     struct dp_rt_l2vxnh_act *nl2vx)
{
  struct dp_rt_l2nh_act *nl2;

  F->tm.tun_rip = nl2vx->rip;
  F->tm.tun_sip = nl2vx->sip;
  F->tm.new_tunnel_id = nl2vx->tid;

  memcpy(&F->il2m, &F->l2m, sizeof(F->l2m));
  F->il2m.vlan[0] = 0;

  nl2 = &nl2vx->l2nh;
  memcpy(F->l2m.dl_dst, nl2->dmac, 6);
  memcpy(F->l2m.dl_src, nl2->smac, 6);
  memcpy(F->pm.lkup_dmac, nl2->dmac, 6);
  F->pm.bd = nl2->bd;
 
  return 0;
}

static int __always_inline
dp_do_nh_lkup(void *ctx, struct xfi *F, void *fa_)
{
  struct dp_nh_key key;
  struct dp_nh_tact *nha;
  int rnh = 0;
#ifdef HAVE_DP_FC
  struct dp_fc_tacts *fa = fa_;
#endif

  key.nh_num = (__u32)F->pm.nh_num;

  LL_DBG_PRINTK("[NHFW] -- Lookup ID %d\n", key.nh_num);
  F->pm.table_id = LL_DP_NH_MAP;

  nha = bpf_map_lookup_elem(&nh_map, &key);
  if (!nha) {
    /* No NH - Drop */
    LLBS_PPLN_DROP(F);
    return 0;
  }

  LL_DBG_PRINTK("[NHFW] action %d pipe %x\n",
                nha->ca.act_type, F->pm.pipe_act);

  if (nha->ca.act_type == DP_SET_DROP) {
    LLBS_PPLN_DROP(F);
  } else if (nha->ca.act_type == DP_SET_TOCP) {
    LLBS_PPLN_TRAP(F);
  } else if (nha->ca.act_type == DP_SET_NEIGH_L2) {
#ifdef HAVE_DP_FC
    struct dp_fc_tact *ta = &fa->fcta[DP_SET_NEIGH_L2];
    ta->ca.act_type = nha->ca.act_type;
    memcpy(&ta->nl2,  &nha->rt_l2nh, sizeof(nha->rt_l2nh));
#endif
    rnh = dp_do_rt_l2_nh(ctx, F, &nha->rt_l2nh);
    /* Check if need to do recursive next-hop lookup */
    if (rnh != 0) {
      nha = bpf_map_lookup_elem(&nh_map, &key);
      if (!nha) {
        /* No NH - Drop */
        LLBS_PPLN_DROP(F);
        return 0;
      }
    }
  } 

  if (nha->ca.act_type == DP_SET_NEIGH_VXLAN) {
#ifdef HAVE_DP_FC
    struct dp_fc_tact *ta = &fa->fcta[DP_SET_NEIGH_VXLAN];
    ta->ca.act_type = nha->ca.act_type;
    memcpy(&ta->nl2vx,  &nha->rt_l2vxnh, sizeof(nha->rt_l2vxnh));
#endif
    return dp_do_rt_l2_vxlan_nh(ctx, F, &nha->rt_l2vxnh);
  }

  return 0;
}

static int __always_inline
dp_eg_l2(void *ctx,  struct xfi *F, void *fa)
{
  /* Any processing based on results from L3 */
  if (F->pm.pipe_act & LLB_PIPE_RDR_MASK) {
    return 0;
  }   
      
  if (F->pm.nh_num != 0) {
    dp_do_nh_lkup(ctx, F, fa);
  }

  dp_do_map_stats(ctx, F, LL_DP_TX_BD_STATS_MAP, F->pm.bd);

  dp_do_dmac_lkup(ctx, F, fa);
  return 0;
}

static int __always_inline
dp_ing_fwd(void *ctx,  struct xfi *F, void *fa)
{
  if (F->l2m.dl_type == bpf_htons(ETH_P_IP)) {
    dp_ing_ipv4(ctx, F, fa);
  }
  return dp_eg_l2(ctx, F, fa);
}

static int __always_inline
dp_ing_l2_top(void *ctx,  struct xfi *F, void *fa)
{
  dp_do_smac_lkup(ctx, F, fa);
  dp_do_tmac_lkup(ctx, F, fa);
  dp_do_tun_lkup(ctx, F, fa);

  if (F->tm.tun_decap) {
    /* FIXME Also need to check if L2 tunnel */
    dp_do_smac_lkup(ctx, F, fa);
    dp_do_tmac_lkup(ctx, F, fa);
  }

  return 0;
}

static int __always_inline
dp_ing_l2(void *ctx,  struct xfi *F, void *fa)
{
  LL_DBG_PRINTK("[ING L2]");
  dp_ing_l2_top(ctx, F, fa);
  return dp_ing_fwd(ctx, F, fa);
}