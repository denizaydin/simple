package main

import (
  "context"
  "flag"
  "reflect"
  "time"

  nmonlistenerapi "./api"
  "github.com/golang/protobuf/ptypes"
  api "github.com/osrg/gobgp/api"
  gobgp "github.com/osrg/gobgp/pkg/server"
  log "github.com/sirupsen/logrus"
  "google.golang.org/grpc"
  "google.golang.org/grpc/codes"
  "google.golang.org/grpc/keepalive"
  "google.golang.org/grpc/status"
)

var (
  nmonGrpcServer = flag.String("nmonlistener_addr", "localhost:10000", "Nmon listerner gRPC address in the format of host:port")
)
var kacp = keepalive.ClientParameters{
  Time:                10 * time.Second, // Send pings every 10 seconds if there is no activity.
  Timeout:             time.Second,      // Wait 1 second for ping ack before considering the connection dead.
  PermitWithoutStream: true,             // Send pings even without active streams.
}

func main() {
  log.SetLevel(log.DebugLevel)
  // Log with timestamp.
  log.SetFormatter(&log.TextFormatter{
    FullTimestamp: true,
  })
  // Do we need more information on logs? look for other options for logrus.
  // log.SetReportCaller(true)
  flag.Parse()

  conn, err := grpc.Dial(*nmonGrpcServer, grpc.WithInsecure(), grpc.WithKeepaliveParams(kacp))

  if err != nil {
    log.Fatalf("did not connect: %v", err)
  }
  defer conn.Close()

  nmonListenerClient := nmonlistenerapi.NewNmonBGPLSServiceClient(conn)

  s := gobgp.NewBgpServer()
  go s.Serve()

  if err := s.StartBgp(context.Background(), &api.StartBgpRequest{
    Global: &api.Global{
      As:         12735,
      RouterId:   "10.2.8.12",
      ListenPort: 179, // gobgp won't listen on tcp:179
    },
  }); err != nil {
    log.Fatal(err)
  }
  if err := s.MonitorPeer(context.Background(), &api.MonitorPeerRequest{}, func(p *api.Peer) { log.Info(p) }); err != nil {
    log.Fatal(err)
  }
  // Neighbor configuration.
  n := &api.Peer{
    Conf: &api.PeerConf{
      NeighborAddress: "10.2.50.226",
      PeerAs:          12735,
    },
    ApplyPolicy: &api.ApplyPolicy{
      ImportPolicy: &api.PolicyAssignment{
        DefaultAction: api.RouteAction_ACCEPT,
      },
      ExportPolicy: &api.PolicyAssignment{
        DefaultAction: api.RouteAction_REJECT,
      },
    },
    AfiSafis: []*api.AfiSafi{
      {
        Config: &api.AfiSafiConfig{
          Family: &api.Family{
            Afi:  api.Family_AFI_LS,
            Safi: api.Family_SAFI_LS,
          },
          Enabled: true,
        },
      },
    },
  }

  if err := s.AddPeer(context.Background(), &api.AddPeerRequest{
    Peer: n,
  }); err != nil {
    log.Fatal(err)
  }
  if err := s.MonitorTable(context.Background(), &api.MonitorTableRequest{
    TableType: api.TableType_GLOBAL,
    Family: &api.Family{
      Afi:  api.Family_AFI_LS,
      Safi: api.Family_SAFI_LS,
    },
  }, func(p *api.Path) {
    /* Hints:
       //For directly accessing path attributes
        fmt.Print(p.NeighborIp)

       //Print type of sth
       fmt.Println(reflect.TypeOf(p).String())
    */
    /*
       Path has Nlri field which is not usable for our scneario!
       We need the NLRI in the path attributes for BGP-LS address familiy.
       Code is from gobgp/internal/pkg/apiutil/attribute.go unmarshalAttribute function line 1345. Not UnmarshalAttribute line 31.
       But there is no information for decoding BGL-LS.
    */
    /*
       Code naming mat not directly give information about the BGP definitions.
    */

    /* The gobgp api LsAttribute attribute do not have a type value to switch on. And the get functions for node, link and prefix tlv do not return nil.
       If we hit mp reach and unreach for bgp-ls address families we search the path attributes again to set required values and prevent unnecessary settings
    */
    /*
       Create node, and link for nmonlistenerapi from bgp-ls address family which will inform their status via mp reach or unreach updates.
       Currently for the gobgp api there is no hash map, or mapped array that we can directly access the path attributes! We have to scan through path attributes.
    */
    /*
       We do not know if its a node,link or prefix (in the current version we do not interested with bgp-ls prefix updates).
       Create empty nmonlistener node and link attribute and status object.
    */

    /*
       Below code get the path attributes and search for update or withdraw. Normally withdraw routes have mpunreach attribute.
       But /orbg/gobgp set api.Path attribute to mpreach for both of update or withdraw routes.
    */

    log.Debugf("New Path, is withdraw:%t", p.IsWithdraw)
    updateType := "mpreach" // true for Mp Reach Update, false for Mp Unreached update, withdraws
    updateFor := "prefix"   // Node, Link Or Prefix
    //Node properties
    Name := ""
    Asn := uint32(0)
    BgpLsInstanceID := uint32(0)
    IgpRouterID := ""
    IsIsArea := int32(0) //actually its bytes

    RouterID := ""
    RouterIDV6 := ""

    IPv4SRCapable := false
    //Link properties

    LocalRouterID := ""
    LocalRouterIDV6 := ""

    LocalAsn := uint32(0)
    LocalBgpLsInstanceID := uint32(0)
    LocalIgpRouterID := ""
    RemoteAsn := uint32(0)
    RemoteBgpLsInstanceID := uint32(0)
    RemoteIgpRouterID := ""
    IgpInterfaceIP := ""
    RemoteNeighborIP := ""
    IgpInterfaceIPv6 := ""
    RemoteNeighborIPv6 := ""
    Bandwidth := float32(0)
    IgpMetric := uint32(0)

    for _, attribute := range p.Pattrs {
      var value ptypes.DynamicAny
      // The type of the attribute is unknown, we can not directly unmarshall
      if err := ptypes.UnmarshalAny(attribute, &value); err != nil {
        log.Errorf(" error during attibute unmarshalling, %s ", err)
      }
      // attribute is still type *any.Any!
      log.Debugf("attribute type is %s", reflect.TypeOf(value.Message).String())
      switch attrib := value.Message.(type) {
      // attrib type is returned message type from ptypes unmarshallAny method
      case *api.MpReachNLRIAttribute:
        log.Debugf("mp reach nlri %s %s", attrib.Family.GetAfi(), attrib.Family.GetSafi())
        // attrib type is api.MpReachNLRIAttribute so we may use its fieds or methods
        for _, anlri := range attrib.Nlris {
          // The type of the attribute nlri is unknown, we can not directly unmarshall
          var anlrivalue ptypes.DynamicAny
          if err := ptypes.UnmarshalAny(anlri, &anlrivalue); err != nil {
            log.Errorf("error during attribute mp reach nlri type unmarshalling %s ", err)
          }
          log.Debugf("attribute mp reach nlri type %s, = %s ", reflect.TypeOf(anlrivalue.Message).String(), anlrivalue.Message)
          switch nlri := anlrivalue.Message.(type) {
          // attrib type is returned message type from ptypes unmarshallAny method
          // the type value may be configusing for bgp-ls nlri actual name. It should be bgl-ls nlri
          case *api.LsAddrPrefix:
            switch nlri.GetType().String() {
            case "LS_NLRI_NODE":
              var LsAddrPrefixNlriValue ptypes.DynamicAny
              if err := ptypes.UnmarshalAny(nlri.GetNlri(), &LsAddrPrefixNlriValue); err != nil {
                log.Errorf("during attribute mp reach nlri, bgp-ls nlri type unmarshalling %s ", err)
              }
              log.Debugf("attribute mp reach nlri, bgp-ls nlri type %s ", reflect.TypeOf(LsAddrPrefixNlriValue.Message).String(), LsAddrPrefixNlriValue.Message)
              switch lsprefix := LsAddrPrefixNlriValue.Message.(type) {
              case *api.LsNodeNLRI:
                // gobgp api do not give bgp-ls protocol id and identifier or instance id from the nlri
                updateFor = "Node"
                Asn = lsprefix.GetLocalNode().GetAsn()
                BgpLsInstanceID = lsprefix.GetLocalNode().GetBgpLsId()
                IgpRouterID = lsprefix.GetLocalNode().GetIgpRouterId()

              }
            case "LS_NLRI_LINK":
              var LsAddrPrefixNlriValue ptypes.DynamicAny
              if err := ptypes.UnmarshalAny(nlri.GetNlri(), &LsAddrPrefixNlriValue); err != nil {
                log.Errorf("during attribute mp reach nlri, bgp-ls nlri type unmarshalling %s ", err)
              }
              log.Debugf("attribute mp reach nlri, bgp-ls nlri type %s ", reflect.TypeOf(LsAddrPrefixNlriValue.Message).String(), LsAddrPrefixNlriValue.Message)
              switch lsprefix := LsAddrPrefixNlriValue.Message.(type) {
              case *api.LsLinkNLRI:
                updateFor = "Link"
                LocalAsn = lsprefix.GetLocalNode().GetAsn()
                LocalBgpLsInstanceID = lsprefix.GetLocalNode().GetBgpLsId()
                LocalIgpRouterID = lsprefix.GetLocalNode().GetIgpRouterId()
                RemoteAsn = lsprefix.GetRemoteNode().GetAsn()
                RemoteBgpLsInstanceID = lsprefix.GetRemoteNode().GetBgpLsId()
                RemoteIgpRouterID = lsprefix.GetRemoteNode().GetIgpRouterId()

                IgpInterfaceIP = lsprefix.GetLinkDescriptor().GetInterfaceAddrIpv4()
                RemoteNeighborIP = lsprefix.GetLinkDescriptor().GetNeighborAddrIpv4()
                IgpInterfaceIPv6 = lsprefix.GetLinkDescriptor().GetInterfaceAddrIpv6()
                RemoteNeighborIPv6 = lsprefix.GetLinkDescriptor().GetNeighborAddrIpv6()

              }
            default:
              log.Warnf("unmachted attibute bgp-ls nlri type, %s", nlri.GetType().String())
            }
          }
        }
      case *api.LsAttribute:
        // There is no type value defined for LsAttribute on the api and Get functions for node,link and prefix do not return nil
        log.Debugf("bgp-ls attribute %s", attrib.String())

        Name = attrib.GetNode().GetName()
        if len(attrib.GetNode().GetIsisArea()) == 1 {
          IsIsArea = int32((attrib.GetNode().GetIsisArea()[0]>>4)*10 + (attrib.GetNode().GetIsisArea()[0] & 0xf))
        }
        RouterID = attrib.GetNode().GetLocalRouterId()
        RouterIDV6 = attrib.GetNode().GetLocalRouterIdV6()
        IPv4SRCapable = attrib.GetNode().GetSrCapabilities().GetIpv4Supported()
        LocalRouterID = attrib.GetLink().GetLocalRouterId()
        LocalRouterIDV6 = attrib.GetLink().GetLocalRouterIdV6()
        Bandwidth = attrib.GetLink().GetBandwidth()
        IgpMetric = attrib.GetLink().GetIgpMetric()

      }
    }
    // send message to the nmon
    if p.IsWithdraw {
      switch updateFor {
      case "Node":
        log.Debugf("%s update for node,\n\tName:%s Asn:%d BGP-LS Instance-ID:%d IGPRouterID:%s RouterID:%s RouterIDv6:%s ISIS Area:%x IPv4SRCapable:%t",
          updateType, Name, Asn, BgpLsInstanceID, IgpRouterID, RouterID, RouterIDV6, IsIsArea, IPv4SRCapable)
        updateNode := &nmonlistenerapi.IgpNode{
          Name:            Name,
          Asn:             Asn,
          BgpLsInstanceId: BgpLsInstanceID,
          IgpRouterId:     IgpRouterID,
          RouterId:        RouterID,
          Status:          2,
        }

        log.Debugf("node is down,\n%s", updateNode.String())
        response, err := nmonListenerClient.SetIgpNode(context.Background(), updateNode)

        if err != nil {
          switch status.Code(err) {
          case codes.DeadlineExceeded:
            log.Errorf("context is expired!, error: %v", status.Code(err))
          case codes.Unavailable:
            log.Errorf("server is unavailable, error: %v", status.Code(err))
          default:
            log.Debugf("unimplemented error: %v", status.Code(err))
          }
        } else {
          log.Debugf("nmon response:\n%s", response.String())
        }
      case "Link":
        log.Debugf("%s update for link,RouterID:%s RouterIDv6:%s\n\tinterface \n\tip:%s neighbor ip:%s\n\tipv6:%s neighbor ipv6:%s \n\tbandwith:%d igp metric:%d\n\tLocal Router\n\t\tAsn:%d BGP-LS InstanceID:%d IGPRouterID:%s\n\tRemote Router\n\t\tAsn:%d BGP-LS InstanceID:%d IGPRouterID:%s",
          updateType, LocalRouterID, LocalRouterIDV6, IgpInterfaceIP, RemoteNeighborIP, IgpInterfaceIPv6, RemoteNeighborIPv6, int(Bandwidth), IgpMetric, LocalAsn, LocalBgpLsInstanceID, LocalIgpRouterID, RemoteAsn, RemoteBgpLsInstanceID, RemoteIgpRouterID)

        updateLink := &nmonlistenerapi.IgpLink{
          LocalAsn:              LocalAsn,
          LocalBgpLsInstanceId:  LocalBgpLsInstanceID,
          LocalIgpRouterId:      LocalIgpRouterID,
          RemoteAsn:             RemoteAsn,
          RemoteBgpLsInstanceId: RemoteBgpLsInstanceID,
          RemoteIgpRouterId:     RemoteIgpRouterID,
          IgpInterfaceIp:        IgpInterfaceIP,
          RemoteNeighborIp:      RemoteNeighborIP,
          IgpInterfaceIpv6:      IgpInterfaceIPv6,
          RemoteNeighborIpv6:    RemoteNeighborIPv6,
          Status:                2,
        }
        log.Debugf("link is down,\n%s", updateLink.String())
        response, err := nmonListenerClient.SetIgpLink(context.Background(), updateLink)
        if err != nil {
          switch status.Code(err) {
          case codes.DeadlineExceeded:
            log.Errorf("context is expired!, error: %v", status.Code(err))
          case codes.Unavailable:
            log.Errorf("server is unavailable, error: %v", status.Code(err))
          default:
            log.Debugf("unimplemented error: %v", status.Code(err))
          }
        } else {
          log.Debugf("nmon response:\n%s", response.String())
        }
      case "Prefix":
      }
    } else {
      switch updateFor {
      case "Node":
        log.Debugf("%s update for node,\n\tName:%s Asn:%d BGP-LS Instance-ID:%d IGPRouterID:%s RouterID:%s RouterIDv6:%s ISIS Area:%x IPv4SRCapable:%t",
          updateType, Name, Asn, BgpLsInstanceID, IgpRouterID, RouterID, RouterIDV6, IsIsArea, IPv4SRCapable)

        updateNode := &nmonlistenerapi.IgpNode{
          Name:            Name,
          Asn:             Asn,
          BgpLsInstanceId: BgpLsInstanceID,
          IgpRouterId:     IgpRouterID,
          RouterId:        RouterID,
          RouterIdV6:      RouterIDV6,
          IsisArea:        IsIsArea,
          IPv4SRCapable:   IPv4SRCapable,
          Status:          0,
        }
        log.Debugf("node is up,\n%s", updateNode.String())

        response, err := nmonListenerClient.SetIgpNode(context.Background(), updateNode)
        if err != nil {
          log.Errorf("%v.ListFeatures(_) = _, %v", nmonListenerClient, err)
        }
        log.Debugf("nmon response:\n%s", response.String())

      case "Link":
        log.Debugf("%s update for link,RouterID:%s RouterIDv6:%s\n\tinterface \n\tip:%s neighbor ip:%s\n\tipv6:%s neighbor ipv6:%s \n\tbandwith:%d igp metric:%d\n\tLocal Router\n\t\tAsn:%d BGP-LS InstanceID:%d IGPRouterID:%s\n\tRemote Router\n\t\tAsn:%d BGP-LS InstanceID:%d IGPRouterID:%s",
          updateType, LocalRouterID, LocalRouterIDV6, IgpInterfaceIP, RemoteNeighborIP, IgpInterfaceIPv6, RemoteNeighborIPv6, int(Bandwidth), IgpMetric, LocalAsn, LocalBgpLsInstanceID, LocalIgpRouterID, RemoteAsn, RemoteBgpLsInstanceID, RemoteIgpRouterID)
        updateLink := &nmonlistenerapi.IgpLink{
          LocalAsn:              LocalAsn,
          LocalBgpLsInstanceId:  LocalBgpLsInstanceID,
          LocalIgpRouterId:      LocalIgpRouterID,
          RemoteAsn:             RemoteAsn,
          RemoteBgpLsInstanceId: RemoteBgpLsInstanceID,
          RemoteIgpRouterId:     RemoteIgpRouterID,
          IgpInterfaceIp:        IgpInterfaceIP,
          RemoteNeighborIp:      RemoteNeighborIP,
          IgpInterfaceIpv6:      IgpInterfaceIPv6,
          RemoteNeighborIpv6:    RemoteNeighborIPv6,
          LocalRouterId:         LocalRouterID,
          LocalRouterIdV6:       LocalRouterIDV6,
          Bandwidth:             Bandwidth,
          IgpMetric:             IgpMetric,
          Status:                0,
        }
        log.Debugf("link is up,\n%s", updateLink.String())
        response, err := nmonListenerClient.SetIgpLink(context.Background(), updateLink)
        if err != nil {
          switch status.Code(err) {
          case codes.DeadlineExceeded:
            log.Errorf("context is expired!, error: %v", status.Code(err))
          case codes.Unavailable:
            log.Errorf("server is unavailable, error: %v", status.Code(err))
          default:
            log.Debugf("unimplemented error: %v", status.Code(err))
          }
        } else {
          log.Debugf("nmon response:\n%s", response.String())
        }
      case "Prefix":
        log.Warn("unimplemented update")
      }
    }
  }); err != nil {
    log.Fatal(err)
  }
  select {} // Block forever; run with GODEBUG=http2debug=2 to observe ping frames and GOAWAYs due to idleness.
}
