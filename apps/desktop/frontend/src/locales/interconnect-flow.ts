// Labels describe the receiving device, not the underlying transport.
export const transferFlowMessages={
 zh:{
  sendOrReceive:'发送与接收',scanToReceive:'扫码接收',receiveMethod:'接收方式',lanDirect:'局域网直连',unifiedSendHint:'选好内容后发送，再把下方二维码或链接发给对方。',
  advancedOptions:'高级选项',sendToShare:'发送到',continueShare:'继续当前分享',newShare:'新建独立分享',addToShare:'添加并发送',
  appendShareHint:'新内容加入同一分享，局域网链接不变。跨网口令传输会重新开始，请使用下方最新入口；更改高级选项请新建独立分享。',
  receivingAndHistory:'接收入口与记录',lanReceive:'局域网直连（可选）',lanReceiveHint:'另一种接收方式：设备在同一局域网时，可直接浏览共享内容。无需经过公网中继。',lanQR:'局域网直连接收二维码',
  internetReceive:'扫码接收（不限网络）',internetReceiveHint:'使用 croc 接收，不要求双方连接同一网络。手机和电脑浏览器都可使用。',internetQR:'跨网网页接收二维码',copyInternetLink:'复制跨网接收链接',copyPhoneLink:'复制局域网链接',
  browserReceiveSteps:'扫码或打开链接后，在 croc 网页确认接收。也可打开 getcroc.com 输入下方口令。发送端须保持在线；一次接收后可点“再次通过口令发送”。',
  nativeCodeOnly:'当前使用自定义中继或仅原生客户端支持的设置，不能通过官方网页接收。请在 WinOne 输入口令，并使用相同网络设置；手机也可使用可选的局域网入口。',
  codeDelivered:'本次跨网传输已送达，局域网与持续分享仍可用。',
  winoneLiveShare:'WinOne 持续分享',nativeShareSteps:'对方在 WinOne → 互联助手 → 文件互传 → 接收中粘贴此地址。此地址不是网页链接，不能直接在手机浏览器使用。双方保持在线即可持续访问，直到停止分享。',copyNativeAddress:'复制 WinOne 分享地址',
  receiveSteps:'粘贴对方的 croc 口令、跨网接收链接，或 WinOne 持续分享地址，再点击连接接收。',
  relayFailureHint:'WinOne 持续分享连接失败，不影响已可用的局域网和 croc 传输。检查网络或中继设置，再停止并恢复分享重试。',
  settingsForNextSend:'这些设置用于新建分享。优先使用指定端口；端口被占用时自动分配可用端口，不会关闭其他程序。',
 },
 en:{
  sendOrReceive:'Send and receive',scanToReceive:'Scan to receive',receiveMethod:'Receive method',lanDirect:'LAN direct',unifiedSendHint:'Choose content and send, then give the recipient the QR code or link below.',
  advancedOptions:'Advanced options',sendToShare:'Send to',continueShare:'Continue sharing',newShare:'New separate share',addToShare:'Add and send',
  appendShareHint:'Content joins the same share and keeps its LAN link. Code transfer restarts: use the latest entry below. Create a separate share to change advanced options.',
  receivingAndHistory:'Receive links and history',lanReceive:'LAN direct (optional)',lanReceiveHint:'An alternative for devices on the same local network: browse shared content directly without a public relay.',lanQR:'LAN direct receive QR code',
  internetReceive:'Scan to receive (any network)',internetReceiveHint:'Receive with croc in a phone or computer browser. Both devices can be on different networks.',internetQR:'Cross-network browser receive QR code',copyInternetLink:'Copy cross-network link',copyPhoneLink:'Copy LAN link',
  browserReceiveSteps:'Scan or open the link, then confirm receipt in croc. Or open getcroc.com and enter the code below. Keep the sender online. After one receipt, choose “Send by code again” for another.',
  nativeCodeOnly:'Custom relay or native-only settings cannot use the official web receiver. Enter the code in WinOne with matching network settings; phones can use the LAN entry instead.',
  codeDelivered:'Code transfer delivered. LAN and live sharing remain available.',
  winoneLiveShare:'WinOne live sharing',nativeShareSteps:'The recipient pastes this address into WinOne → Interconnect → File transfer → Receive. It is not a web link and cannot be used directly in a phone browser. Both sides stay online until sharing stops.',copyNativeAddress:'Copy WinOne share address',
  receiveSteps:'Paste a croc code, cross-network receive link, or WinOne live share address, then connect to receive.',
  relayFailureHint:'WinOne live sharing could not connect. Available LAN and croc transfers are unaffected. Check the network or relay settings, then stop and resume the share to retry.',
  settingsForNextSend:'Used for new shares. The selected port is preferred; if busy, a free port is assigned without stopping other applications.',
 }
};
