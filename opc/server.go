package opc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
)

type OpcClient struct {
	Endpoint string
	Duration time.Duration
	client   *opcua.Client
	sub      *opcua.Subscription
	ctx      context.Context
	cancel   context.CancelFunc
	gateway  chan Data
	Nodes    []NodeId
	Username string
	Password string
}

type NodeId struct {
	ID   uint64
	Node string
}

type Data struct {
	ID         uint64      `json:"id"`
	DataType   string      `json:"dataType"`
	Value      interface{} `json:"value"`
	SourceTime time.Time   `json:"sourceTime"`
	Param      string      `json:"param"`
}

type TreeNode struct {
	NodeID     *ua.NodeID  `json:"nodeId"`
	BrowseName string      `json:"browseName"`
	Children   []*TreeNode `json:"children"`
}

var a = 0

func (o *OpcClient) Start() {
	for {
		err := o.connect()
		if err != nil {
			log.Printf("连接失败 [%s]，5秒后重试: %v", o.Endpoint, err)
			time.Sleep(5 * time.Second)
			continue
		}

		// 如果 connect 成功，阻塞直到断开
		err = o.monitor()
		log.Printf("连接中断 [%s]: %v，5秒后重连...", o.Endpoint, err)
		time.Sleep(5 * time.Second)
	}
}

func (o *OpcClient) connect() (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()
	o.cancel = cancel
	o.ctx = ctx
	endpoints, err := opcua.GetEndpoints(ctx, o.Endpoint)
	if err != nil {
		// panic(err)
		return
	}

	ep, err := opcua.SelectEndpoint(endpoints, ua.SecurityPolicyURINone, ua.MessageSecurityModeNone)
	// ep, err := opcua.SelectEndpoint(endpoints, ua.SecurityPolicyURIBasic256Sha256, ua.MessageSecurityModeSignAndEncrypt)

	if err != nil {
		log.Fatal(err)
		return
	}
	ep.EndpointURL = o.Endpoint

	/**

		 opcua.SecurityPolicy("None"),
	    opcua.SecurityMode(ua.MessageSecurityModeNone),
	    opcua.AuthAnonymous(),
	*/
	opts := []opcua.Option{
		opcua.SecurityPolicy("None"),                   // 设置为无安全策略
		opcua.SecurityMode(ua.MessageSecurityModeNone), // 设置为无消息安全模式
		// opcua.CertificateFile(""),
		// opcua.PrivateKeyFile(""),
		// opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous), // 从 endpoint 自动提取安
	}

	if o.Username != "" && o.Password != "" {
		fmt.Println("使用用户名密码连接", o.Username, o.Password, o.Endpoint)
		// opts = append(opts, opcua.AuthUsername(o.Username, o.Password), opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeUserName))
		opts = append(opts, opcua.AuthUsername(o.Username, o.Password), opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeUserName))

	} else {
		//  opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous)
		opts = append(opts, opcua.AuthAnonymous())
	}

	c, err := opcua.NewClient(ep.EndpointURL, opts...)
	if err != nil {
		log.Fatal(err)
		return
	}
	if err = c.Connect(ctx); err != nil {
		log.Fatal(err)
		return
	}
	fmt.Printf("连接成功%s\n", o.Endpoint)
	// defer c.Close(ctx)

	o.client = c
	return
	// }()
}

func (o *OpcClient) monitor() (err error) {
	defer func() {
		if o.client != nil {
			o.client.Close(o.ctx)
		}
	}()
	// // 先从opc服务器获取所有节点
	rootNode := ua.NewNumericNodeID(0, id.ObjectsFolder)
	nodeIDs := browseNodeTree(o.ctx, o.client, rootNode)
	u, err := url.Parse(o.Endpoint)
	if err != nil {
		return
	}
	// 写入json文件
	f, err := os.OpenFile("/www/opc/"+u.Host+".json", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	a++
	if err != nil {
		log.Fatal(err)
		return
	}
	defer f.Close()
	jsonData, err := json.Marshal(nodeIDs)
	if err != nil {
		log.Fatal(err)
		return
	}
	f.Write(jsonData)

	exitIds := []NodeId{}
	nodeExitIds := []string{}
	treeNodeIDs := flattenTreeNodeIDs(nodeIDs)
	fmt.Println("节点数量", len(o.Nodes))

	for _, n := range o.Nodes {
		// exitIds = append(exitIds, n)
		var isExit bool
		for _, id := range treeNodeIDs {
			if n.Node == id {
				isExit = true
				exitIds = append(exitIds, n)
				break
			}
		}
		if !isExit {
			nodeExitIds = append(nodeExitIds, n.Node)
		}
	}
	if len(nodeExitIds) > 0 {
		fmt.Println("不存在的nodeId", nodeExitIds)
		f2, _ := os.OpenFile("/www/opc/"+"notexit.json", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		notJson, _ := json.Marshal(nodeExitIds)
		f2.Write(notJson)
		f2.Close()
	}

	// ========================================不可订阅

	o.Nodes = exitIds
	fmt.Println("环境档案ID222", len(o.Nodes))
	notifyCh := make(chan *opcua.PublishNotificationData, 3000)

	sub, err := o.client.Subscribe(o.ctx, &opcua.SubscriptionParameters{
		Interval:                   10 * time.Second,
		MaxKeepAliveCount:          1,
		LifetimeCount:              0,
		MaxNotificationsPerPublish: 0,
		Priority:                   0,
	}, notifyCh)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer sub.Cancel(o.ctx)

	// pollInterval := 10 * time.Second // 轮询间隔

	// //  轮训获取节点数据

	// for {
	// 	select {
	// 	case <-o.ctx.Done():
	// 		return
	// 	case <-time.After(pollInterval):
	// 		for _, n := range o.Nodes {

	// 			fmt.Println("环境档案ID", n.ID)
	// 			nodeID, err := ua.ParseNodeID(n.Node)

	// 			if err != nil {
	// 				fmt.Println("环境档案ID解析 NodeID 失败:", n.Node, err)
	// 				continue
	// 			}

	// 			node := o.client.Node(nodeID)

	// 			// 读取节点值
	// 			val, err := node.Value(o.ctx)
	// 			if err != nil {
	// 				fmt.Println("环境档案ID读取节点值失败:", n.Node, err)
	// 				continue
	// 			}

	// 			if val == nil || val.Value() == nil {
	// 				fmt.Println("环境档案ID节点值为空:", n.Node)
	// 				continue
	// 			}

	// 			// 封装数据
	// 			data := Data{
	// 				ID:         uint64(n.ID),
	// 				DataType:   val.Type().String(),
	// 				Value:      val.Value(),
	// 				SourceTime: val.Time(),
	// 			}
	// 			fmt.Println("环境档案ID发送", data)

	// 			// 发送到网关通道
	// 			select {
	// 			case o.gateway <- data:
	// 				fmt.Println("发送到注册网关:", n.Node, data.Value)
	// 			default:
	// 				fmt.Println("阻塞环境档案ID发送", data)
	// 			}
	// 		}

	// 		time.Sleep(pollInterval)
	// 	}
	// }

	cannotSubscribe := []string{}
	mon := []*ua.MonitoredItemCreateRequest{}

	for _, n := range exitIds {
		nodeID, err := ua.ParseNodeID(n.Node)
		if err != nil {
			log.Println("解析 NodeID 失败:", n.Node, err)
			continue
		}

		// node := o.client.Node(nodeID)
		// class, err := node.NodeClass(o.ctx)
		// if err != nil {
		// 	log.Println("获取 NodeClass 失败:", n.Node, err)
		// 	cannotSubscribe = append(cannotSubscribe, n.Node)
		// 	continue
		// }

		// if class != ua.NodeClassVariable {
		// 	// 不是 Variable 类型，不可订阅
		// 	cannotSubscribe = append(cannotSubscribe, n.Node)
		// 	continue
		// }

		mi := o.valueRequest(nodeID, uint32(n.ID))
		mon = append(mon, mi)
	}
	if len(cannotSubscribe) > 0 {
		fmt.Println("不可订阅的节点", cannotSubscribe)
		f3, _ := os.OpenFile("/www/opc/"+"cannotsubscribe.json", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		cannotSubscribeJson, _ := json.Marshal(cannotSubscribe)
		f3.Write(cannotSubscribeJson)
		f3.Close()
	}
	// ========================================不可订阅

	fmt.Println("订阅节点环境档案", len(mon))
	r, err := sub.Monitor(o.ctx, ua.TimestampsToReturnBoth, mon...)
	if err != nil {
		fmt.Println("订阅失败", err)
		return
	}
	if r != nil && len(r.Results) > 0 {
		for _, res := range r.Results {
			if res.StatusCode != ua.StatusOK {
				fmt.Println("订阅失败", res.StatusCode)
				return
			}
		}
	}

	o.sub = sub

	// go func() {
	for {
		select {
		case <-o.ctx.Done():
			{
				// 重新连接
				return fmt.Errorf("短线了")
			}
		case res := <-notifyCh:
			fmt.Printf("Received publish notification: %v\n", res)
			if res.Error != nil {
				log.Print(res.Error)
				continue
			}

			switch x := res.Value.(type) {
			case *ua.DataChangeNotification:
				for _, item := range x.MonitoredItems {
					if item.Value == nil {
						fmt.Println("item.Value == nil")
						continue
					}
					// 打印值
					if item.Value.Value == nil {
						fmt.Println("item.Value.Value == nil")
						continue
					}
					// 打印值
					data := item.Value.Value.Value()
					log.Printf("收到服务端数据 %v = %v ", item.ClientHandle, data)
					if item.Value != nil {
						data := Data{
							ID:         uint64(item.ClientHandle),
							DataType:   item.Value.Value.Type().String(),
							Value:      item.Value.Value.Value(),
							SourceTime: item.Value.SourceTimestamp,
						}
						// 判断gateway是否关闭
						select {
						case o.gateway <- data:
							fmt.Println("发送到注册网关==")
						default:
						}
					}
				}

			case *ua.EventNotificationList:
				// for _, item := range x.Events {
				// 	log.Printf("Event for client handle: %v\n", item.ClientHandle)
				// 	for i, field := range item.EventFields {
				// 		log.Printf("%v: %v of Type: %T", eventFieldNames[i], field.Value(), field.Value())
				// 	}
				// 	log.Println()
				// }

			default:
				log.Printf("what's this publish result? %T", res.Value)
			}
		}
	}
}

// browseNodes 递归浏览节点并收集 NodeID
// func browseNodes(ctx context.Context, client *opcua.Client, nodeID *ua.NodeID) []*ua.NodeID {
// 	var nodeIDs []*ua.NodeID

// 	// 添加当前 NodeID
// 	nodeIDs = append(nodeIDs, nodeID)

// 	// 创建 Browse 请求
// 	req := &ua.BrowseRequest{
// 		NodesToBrowse: []*ua.BrowseDescription{
// 			{
// 				NodeID:          nodeID,
// 				BrowseDirection: ua.BrowseDirectionForward,
// 				ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
// 				IncludeSubtypes: true,
// 				NodeClassMask:   uint32(ua.NodeClassAll),
// 				ResultMask:      uint32(ua.BrowseResultMaskAll),
// 			},
// 		},
// 	}

// 	// 执行 Browse 操作
// 	resp, err := client.Browse(ctx, req)
// 	if err != nil {
// 		log.Printf("Failed to browse node %s: %v", nodeID, err)
// 		return nodeIDs
// 	}

// 	// 处理 Browse 结果
// 	for _, result := range resp.Results {
// 		for _, ref := range result.References {
// 			// 获取子节点的 NodeID
// 			childNodeID := ref.NodeID.NodeID
// 			// 递归浏览子节点
// 			childNodeIDs := browseNodes(ctx, client, childNodeID)
// 			nodeIDs = append(nodeIDs, childNodeIDs...)
// 		}
// 	}

// 	return nodeIDs
// }

func browseNodeTree(ctx context.Context, client *opcua.Client, nodeID *ua.NodeID) *TreeNode {
	// 创建当前节点对象
	node := &TreeNode{
		NodeID: nodeID,
	}

	// 创建 Browse 请求
	req := &ua.BrowseRequest{
		NodesToBrowse: []*ua.BrowseDescription{
			{
				NodeID:          nodeID,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
				IncludeSubtypes: true,
				NodeClassMask:   uint32(ua.NodeClassAll),
				ResultMask:      uint32(ua.BrowseResultMaskAll),
			},
		},
	}

	// 执行 Browse 操作
	resp, err := client.Browse(ctx, req)
	if err != nil {
		log.Printf("Failed to browse node %s: %v", nodeID, err)
		return node
	}

	// 递归构建子树
	for _, result := range resp.Results {
		for _, ref := range result.References {
			child := browseNodeTree(ctx, client, ref.NodeID.NodeID)
			child.BrowseName = ref.BrowseName.Name
			node.Children = append(node.Children, child)
		}
	}

	return node
}

func flattenTreeNodeIDs(node *TreeNode) []string {
	var ids []string
	var walk func(n *TreeNode)
	walk = func(n *TreeNode) {
		if n == nil || n.NodeID == nil {
			return
		}
		ids = append(ids, n.NodeID.String())
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(node)
	return ids
}

func (o *OpcClient) AddNodeID(n NodeId) error {
	if o.sub == nil {
		return fmt.Errorf("订阅未初始化，请先调用 Connect")
	}

	id, err := ua.ParseNodeID(n.Node)
	if err != nil {
		return err
	}
	miCreateRequest := o.valueRequest(id, uint32(n.ID))
	// 判断ctx是否关闭
	if o.ctx.Err() != nil {
		return fmt.Errorf("context is done")
	}

	res, err := o.sub.Monitor(o.ctx, ua.TimestampsToReturnBoth, miCreateRequest)
	if err != nil || res.Results[0].StatusCode != ua.StatusOK {
		return err
	}
	o.Nodes = append(o.Nodes, n)
	log.Printf("Added new monitored item for NodeID: %s", n.Node)
	return nil
}

func (o *OpcClient) GetClient() *opcua.Client {
	return o.client
}

func (o *OpcClient) valueRequest(nodeID *ua.NodeID, handle uint32) *ua.MonitoredItemCreateRequest {
	// handle := uint32(42)

	// filter := &ua.DataChangeFilter{
	// 	Trigger:       ua.DataChangeTriggerStatusValueTimestamp, // 始终触发
	// 	DeadbandType:  uint32(ua.DeadbandTypeNone),              // 不使用死区
	// 	DeadbandValue: 0,
	// }

	// // 封装为扩展对象
	// filterExt := ua.NewExtensionObject(filter)

	// // 设置监控参数
	// params := &ua.MonitoringParameters{
	// 	ClientHandle:     handle,
	// 	SamplingInterval: 1000, // 每 30 秒采样一次
	// 	Filter:           filterExt,
	// 	QueueSize:        10, // 保留最新一条
	// 	DiscardOldest:    true,
	// }

	// // 构建监控请求
	// return &ua.MonitoredItemCreateRequest{
	// 	ItemToMonitor: &ua.ReadValueID{
	// 		NodeID:      nodeID,
	// 		AttributeID: ua.AttributeIDValue,
	// 	},
	// 	MonitoringMode:      ua.MonitoringModeReporting,
	// 	RequestedParameters: params,
	// }

	// ===================================================

	// params := &ua.MonitoringParameters{
	// 	ClientHandle:     handle,
	// 	SamplingInterval: 3000, // 3 秒采样
	// 	QueueSize:        10,
	// 	DiscardOldest:    true,
	// 	Filter:           nil, // 先不要 Filter
	// }

	// mi := &ua.MonitoredItemCreateRequest{
	// 	ItemToMonitor: &ua.ReadValueID{
	// 		NodeID:      nodeID,
	// 		AttributeID: ua.AttributeIDValue,
	// 	},
	// 	MonitoringMode:      ua.MonitoringModeReporting,
	// 	RequestedParameters: params,
	// }
	// ========================================
	// // 构建监控请求

	mi := opcua.NewMonitoredItemCreateRequestWithDefaults(nodeID, ua.AttributeIDValue, handle)
	mi.RequestedParameters.SamplingInterval = 10000
	mi.RequestedParameters.QueueSize = 2
	mi.RequestedParameters.DiscardOldest = false

	return mi
}

func eventRequest(nodeID *ua.NodeID) (*ua.MonitoredItemCreateRequest, []string) {
	fieldNames := []string{"EventId", "EventType", "Severity", "Time", "Message"}
	selects := make([]*ua.SimpleAttributeOperand, len(fieldNames))

	for i, name := range fieldNames {
		selects[i] = &ua.SimpleAttributeOperand{
			TypeDefinitionID: ua.NewNumericNodeID(0, id.BaseEventType),
			BrowsePath:       []*ua.QualifiedName{{NamespaceIndex: 0, Name: name}},
			AttributeID:      ua.AttributeIDValue,
		}
	}

	wheres := &ua.ContentFilter{
		Elements: []*ua.ContentFilterElement{
			{
				FilterOperator: ua.FilterOperatorGreaterThanOrEqual,
				FilterOperands: []*ua.ExtensionObject{
					{
						EncodingMask: 1,
						TypeID: &ua.ExpandedNodeID{
							NodeID: ua.NewNumericNodeID(0, id.SimpleAttributeOperand_Encoding_DefaultBinary),
						},
						Value: ua.SimpleAttributeOperand{
							TypeDefinitionID: ua.NewNumericNodeID(0, id.BaseEventType),
							BrowsePath:       []*ua.QualifiedName{{NamespaceIndex: 0, Name: "Severity"}},
							AttributeID:      ua.AttributeIDValue,
						},
					},
					{
						EncodingMask: 1,
						TypeID: &ua.ExpandedNodeID{
							NodeID: ua.NewNumericNodeID(0, id.LiteralOperand_Encoding_DefaultBinary),
						},
						Value: ua.LiteralOperand{
							Value: ua.MustVariant(uint16(0)),
						},
					},
				},
			},
		},
	}

	filter := ua.EventFilter{
		SelectClauses: selects,
		WhereClause:   wheres,
	}

	filterExtObj := ua.ExtensionObject{
		EncodingMask: ua.ExtensionObjectBinary,
		TypeID: &ua.ExpandedNodeID{
			NodeID: ua.NewNumericNodeID(0, id.EventFilter_Encoding_DefaultBinary),
		},
		Value: filter,
	}

	handle := uint32(42)
	req := &ua.MonitoredItemCreateRequest{
		ItemToMonitor: &ua.ReadValueID{
			NodeID:       nodeID,
			AttributeID:  ua.AttributeIDEventNotifier,
			DataEncoding: &ua.QualifiedName{},
		},
		MonitoringMode: ua.MonitoringModeReporting,
		RequestedParameters: &ua.MonitoringParameters{
			ClientHandle:     handle,
			DiscardOldest:    true,
			Filter:           &filterExtObj,
			QueueSize:        10,
			SamplingInterval: 1.0,
		},
	}

	return req, fieldNames
}
