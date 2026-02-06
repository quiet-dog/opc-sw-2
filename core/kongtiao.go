package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sw/global"
	"sw/model/node"
	"sw/model/service"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

type KongTiaoDTO struct {
	DeviceSn                       string  `json:"deviceSn"`
	ZhiBanGongKuanYaLiSheDing      float64 `json:"zhiBanGongKuanYaLiSheDing"`
	ZhiBanGongKuanFengLiangSheDing float64 `json:"zhiBanGongKuanFengLiangSheDing"`
	FengFaWenDingZhuangTai         int16   `json:"fengFaWenDingZhuangTai"`
	FaWeiFanKuan                   int16   `json:"faWeiFanKuan"`
	QiangZhiFaWeiSheDing           int16   `json:"qiangZhiFaWeiSheDing"`
	QiangZhiMoShiKaiGuan           int16   `json:"qiangZhiMoShiKaiGuan"`
	PidKongZhiJiFenXiShu           int16   `json:"pidKongZhiJiFenXiShu"`
	PodKongZhiBiLiXiShu            int16   `json:"podKongZhiBiLiXiShu"`
	FengLiangFanKui                int16   `json:"fengLiangFanKui"`
	FangJianShiJiYaLi              float64 `json:"fangJianShiJiYaLi"`
	GongKuangMoShi                 int16   `json:"gongKuangMoShi"`
	ShuangGongKuangQieHuanShiJian  int16   `json:"shuangGongKuangQieHuanShiJian"`
	FengLiangSheDing               int16   `json:"fengLiangSheDing"`
	YaLiSheDing                    float64 `json:"yaLiSheDing"`
}

func InitKongTiao() {

	var senserService service.ServiceModel
	global.DB.Where("id = ?", 4).First(&senserService)

	client := resty.New().SetTimeout(3 * time.Second).SetBaseURL("http://127.0.0.1:9020").SetAuthToken("MASTER_TOKEN_123456")

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		// endpoint := fmt.Sprintf("opc.tcp://%s", senserService.Opc)
		ctx := context.Background()
		// opc创建连接
		opcClient, err := opcua.NewClient(senserService.Opc,
			opcua.SecurityMode(ua.MessageSecurityModeNone),
			opcua.SecurityPolicy(ua.SecurityPolicyURINone),
			opcua.AutoReconnect(true),
		)
		if err != nil {
			panic(err)
		}

		if err := opcClient.Connect(ctx); err != nil {
			log.Fatalf("❌ 连接失败: %v", err)
		}
		// defer client.Close(ctx)

		for i := 0; i < 17; i++ {
			var nodes []*node.NodeModel
			data := map[string]any{}
			data["deviceSn"] = fmt.Sprintf("%d", i)
			data["isOnline"] = true
			if i < 15 {
				data["deviceType"] = "空调设备"
				global.DB.Where("device_type = ?", "空调设备").Where("device_name = ?", fmt.Sprintf("%d", i)).Find(&nodes)
				for _, n := range nodes {
					nodeIdStr := n.NodeId
					// 读取对应节点的id
					id, err := ua.ParseNodeID(nodeIdStr)
					if err != nil {
						fmt.Println("===============空调读取10", err)
						continue
					}

					req := &ua.ReadRequest{
						MaxAge:             10000,
						TimestampsToReturn: ua.TimestampsToReturnBoth,
						NodesToRead: []*ua.ReadValueID{
							{
								NodeID: id,
							},
						},
					}
					var resp *ua.ReadResponse

					if opcClient != nil {
						resp, err = opcClient.Read(context.Background(), req)
						if err != nil {
							fmt.Println("===============空调读取2", err)
							data["isOnline"] = false
							continue
						}

						switch {
						case err == io.EOF && opcClient.State() != opcua.Closed:
							fmt.Println("===============空调读取3")
							// has to be retried unless user closed the connection
							continue

						case errors.Is(err, ua.StatusBadSessionIDInvalid):
							fmt.Println("===============空调读取4")
							// Session is not activated has to be retried. Session will be recreated internally.
							continue

						case errors.Is(err, ua.StatusBadSessionNotActivated):
							fmt.Println("===============空调读取5")
							// Session is invalid has to be retried. Session will be recreated internally.
							continue

						case errors.Is(err, ua.StatusBadSecureChannelIDInvalid):
							fmt.Println("===============空调读取6")
							// secure channel will be recreated internally.
							continue

						default:
							fmt.Println("=============空调读取失败", err)
						}
						if resp != nil && resp.Results[0].Status != ua.StatusOK {
							data["isOnline"] = false
							fmt.Println("===============空调读取8")
							continue
						}
						val := resp.Results[0].Value.Value()
						fmt.Println("===============空调读取9", val)
						if n.Key == "gongKuangMoShi" {
							// 如果val是1-生产工况；2-值班工况
							if val == 1 {
								data[n.Key] = "生产工况"
							} else {
								data[n.Key] = "值班工况"
							}
						}
						data[n.Key] = val
					}

				}
			}

			if i == 16 {
				data["deviceType"] = "回风机"
				global.DB.Where("device_type = ?", "回风机").Where("device_name = ?", fmt.Sprintf("%d", i)).Find(&nodes)
				for _, n := range nodes {
					nodeIdStr := n.NodeId
					// 读取对应节点的id
					id, err := ua.ParseNodeID(nodeIdStr)
					if err != nil {
						continue
					}

					req := &ua.ReadRequest{
						MaxAge:             10000,
						TimestampsToReturn: ua.TimestampsToReturnBoth,
						NodesToRead: []*ua.ReadValueID{
							{
								NodeID: id,
							},
						},
					}
					var resp *ua.ReadResponse
					// c := opcClient.GetClient()
					if opcClient != nil {
						resp, err = opcClient.Read(context.Background(), req)
						if err != nil {
							data["isOnline"] = false
							continue
						}

						switch {
						case err == io.EOF && opcClient.State() != opcua.Closed:
							// has to be retried unless user closed the connection
							continue

						case errors.Is(err, ua.StatusBadSessionIDInvalid):
							// Session is not activated has to be retried. Session will be recreated internally.
							continue

						case errors.Is(err, ua.StatusBadSessionNotActivated):
							// Session is invalid has to be retried. Session will be recreated internally.
							continue

						case errors.Is(err, ua.StatusBadSecureChannelIDInvalid):
							// secure channel will be recreated internally.
							continue

						default:
							// log.Fatalf("Read failed: %s", err)
							fmt.Println("===============回风机读取1", err)
						}
						if resp != nil && resp.Results[0].Status != ua.StatusOK {
							data["isOnline"] = false
							continue
						}
						val := resp.Results[0].Value.Value()
						switch n.Key {
						case "huiFengJiShouZiDong":
							if val == 1 {
								data[n.Key] = "自动"
							} else {
								data[n.Key] = "手动"
							}
						case "huiFengJiGuZhang":
							if val == 1 {
								data[n.Key] = "故障"
							} else {
								data[n.Key] = "无故障"
							}
						case "huiFengJiYunXing":
							if val == 1 {
								data[n.Key] = "运行"
							} else {
								data[n.Key] = "停止"
							}
						case "huiFengMiBiKaiGuanKongZhi":
							if val == 1 {
								data[n.Key] = "开控制"
							} else {
								data[n.Key] = "关控制"
							}
						case "huiFengMiBiGuanDaoWei", "huiFengMiBiKaiDaoWei", "yuanXinFengKouZengJiaXinFengFaGuanDaoWei", "zengJiaXinFengKouXinFengFaGuanDaoWei", "yuanXinFengKouZengJiaXinFengFaKaiDaoWei", "zengJiaXinFengKouXinFengFaKaiDaoWei":
							if val == 1 {
								data[n.Key] = "关到位"
							} else {
								data[n.Key] = "没有关到位"
							}
						case "huiFengJiQiTing":
							if val == 1 {
								data[n.Key] = "开控制"
							} else {
								data[n.Key] = "关控制"
							}
						case "yuanXinFengKouZengJiaXinFengFaKaiGuanKongZhi", "zengJiaXinFengKouXinFengFaKaiGuanKongZhi":
							if val == 1 {
								data[n.Key] = "开控制"
							} else {
								data[n.Key] = "关控制"
							}
						case "moShiQieHuan":
							if val == 0 {
								data[n.Key] = "全新风模式"
							} else {
								data[n.Key] = "回风模式"
							}
						case "gongKuangQieHuan":
							if val == 0 {
								data[n.Key] = "生产工况"
							} else {
								data[n.Key] = "值班工况"
							}
						default:
							data[n.Key] = val
						}

					}
				}
			}
			fmt.Println("============空调数据发送")
			fmt.Println(data)
			// 传输json
			client.R().SetHeader("Content-Type", "application/json").SetBody(data).Post("/manage/kongTiaoData")
			msg := global.RecHandler{}
			msg.Type = global.KONGTIAO
			msg.Data = data
			global.RecChanel <- msg
		}
		opcClient.Close(ctx)
	}

}
