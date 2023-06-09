[TOC]
## Socks5 Server 实现
强烈建议阅读：[理解socks5协议的工作过程和协议细节](https://wiyi.org/socks5-protocol-in-deep.html)
### 介绍
socks是一种互联网协议，它通过一个代理服务器在客户端和服务端之间交换网络数据。简单来说，它就是一种代理协议，扮演一个中间人的角色，在客户端和目标主机之间转发数据。

socks协议位于OSI模型中的第五层，即会话层(Session Layer)。
### 原理
我们来写一个 socks5 代理服务器，对于大家来说，一提到代理服务器，第一想到的是翻墙。不过很遗憾的是，socks5 协议它虽然是代理协议，但它并不能用来翻墙，它的协议都是明文传输。
这个协议历史比较久远，诞生于互联网早期。它的用途是，比如某些企业的内网为了确保安全性，有很严格的防火墙策略，但是带来的副作用就是访问某些资源会很麻烦。
socks5 相当于在防火墙开了个口子，让授权的用户可以通过单个端口去访问内部的所有资源。实际上很多翻墙软件，最终暴露的也是一个 socks5 协议的端口。
如果有同学开发过爬虫的话，就知道，在爬取过程中很容易会遇到IP访问频率超过限制。这个时候很多人就会去网上找一些代理IP池，这些代理IP池里面的很多代理的协议就是 socks5。
socks5顾名思义就是socks协议的第五个版本，作为socks4的一个延伸，在socks4的基础上新增UDP转发和认证功能。唯一遗憾的是socks5并不兼容socks4协议。socks5由IETF在1996年正式发布，经过这么多年的发展，互联网上基本上都以socks5为主，socks4已经退出了历史的舞台。

我们先来看一下最终写完的代理服务器的效果。
我们启动这个程序，然后在浏览器里面配置使用这个代理，此时我们打开网页。
代理服务器的日志，会打印出你访问的网站的域名或者 IP ，这说明我们的网络流量是通过这个代理服务器的。
我们也能在命令行去测试我们的代理服务器。我们可以用 curl --socks5 + 代理服务器地址，后面加个可访问的 URL，如果代理服务器工作正常的话，那么 curl 命令就会正常返回。

![Alt text](images/expl.png)

接下来我们来了解一下 socks5 协议的工作原理。正常浏览器访问一个网站，如果不经过代理服务器的话，就是先和对方的网站建立 TCP 连接，然后三次握手，握手完之后发起 HTTP 请求，然后服务返回 HTTP 响应。如果设置代理服务器之后，流程会变得复杂一些。
首先是浏览器和 socks5 代理建立 TCP 连接，代理再和真正的服务器建立 TCP 连接。这里可以分成四个阶段，握手阶段、认证阶段、请求阶段、relay 阶段。
第一个握手阶段，浏览器会向 socks5 代理发送请求，包的内容包括一个协议的版本号，还有支持的认证的种类，socks5 服务器会选中一个认证方式，返回给浏览器。如果返回的是 00 的话就代表不需要认证，返回其他类型的话会开始认证流程，这里我们就不对认证流程进行概述了。

![Alt text](images/step.png)

### 简易 TCP echo Server
第一步，我们先在 go 里面写一个简单的 TCP echo server，为了方便测试，server 的工作逻辑很简单，你给他发送啥，他就回复啥，代码：[TCP echo server](echo-server.go)
首先在 main 函数里面先用 net.listen 去监听一个端口，会返回一个server，然后在一个死循环里面，每次去 accept 一个请求，成功就会返回一个连接。接下来的话我们在一个process函数里面去处理这个连接。
注意这前面会有个 go 关键字，这个代表启动一 go routinue，可以暂时类比为其他语言里面的启动一个子线程，只是这里的 go routinue 的开销会比子线程要小很多，可以很轻松地处理上万的开发，
接下来是这个 process 函数实现，首先第一步会先加一个 defer connection.close()，defer是 Golang 里面的一个语法，这一行的含义就是代表在这个函数退出的时候要把这个连接关掉，否则会有资源的泄露。
接下来我们用 bufio.NewReader 来创建一个带缓冲的只读流，这个在前面的猜谜游戏里面也有用到，带缓冲的流的作用是，可以减少底层系统调用的次数，比如这里为了方便是一个字节一个字节的读取，但是底层可能合并成几次大的读取操作，并且带缓冲的流会有更多的一些工具函数用来读取数据
我们可以简单地调用那个 readbyte 函数来读取单个字节，再把这一个字节写进去连接。
##### test echo-server:
```bash
$ go run echo-server.go
# change to another tab
$ nc 127.0.0.1 1080
```

### Auth 认证阶段
就这样我们就已经完成了一个能够返回你输入信息的一个 TCP server ，接下来我们是要开始实现协议的第一步，认证阶段，从这一部分开始会变得比较复杂。我们实现一个空的 auth 函数，在 process 函数里面调用，再来编写 auth 函数的代码我们回忆一下认证阶段的逻辑，首先第一步的话，浏览器会给代理服务器发送一个包，然后这个包有三个字段，第一个字段，version 也就是 协议版本号 ，固定是 5 第二个字段 methods，认证的方法数目第二个字段 每个 method的编码， 0代表 不需要认证，2 代表用户名密码认证我们先用 read bytes 来把版本号读出来，然后如果版本号不是 socket 5 的话直接返回报错，接下来我们再读取 method size，也是一个字节。然后我们需要我们去 make 一人相应长度的一个slice ，用 io.ReadFull 把它去填充进去。

此时curl命令肯定是不成功的，因为协议还没实现完成。但是看日志发现，version和method可以正常打印，说明当前的实现是正确的。

##### test
curl测试socks5代理命令（注意在cmd下）
```bash
$ curl --socks5 127.0.0.1:1080 -v https://www.qq.com
```

### 请求阶段

接下来我们开始做第三步，实现请求阶段，我们试图读取到 携带 URL 或者 IP 地址+端口的包，然后把它打印出来。我们实现一个和 auth 函数类似的 connect 函数，同样在 process 里面去调用。再来实现 connect 函数的代码。
回忆一下请求阶段的逻辑，浏览器会发送一个包，包里面包含如下6个字段，version 版本号，还是 5。command，代表请求的类型，我们只支持 connection 请求，也就是让代理服务建立新的TCP连接。RSV 保留字段，不理会。atype 就是目标地址类型，可能是 IPv4 IPv6 或者域名，下面是 addr，这个地址的长度是根据 atype 的类型而不同的，port 端口号，两个字节，我们需要逐个去读取这些字段。

前面这四个字段总共四个字节，我们可以一次性把它读出来。我们定义一个长度为4的buffer 然后把它读满。读满之后，然后第0个、第1个、第3个，分别是 version，cmd 和 type,version 需要判断是 socks5，cmd 需要判断是 1。下面的 atype，可能是ipv4，ipv6，或者是 host。
最后还有两个字节是 port，我们读取它，然后按协议规定的大端字节序转换成数字。由于上面的 buffer 已经不会被其他变量使用了，我们可以直接复用之前的内存，建立一个临时的 slice ，长度是2，用于读取，这样的话最多会只读两个字节回来。接下来我们把这个地址和端口打印出来用于调试。收到浏览器的这个请求包之后，我们需要返回一个包，这个包有很多字段，但其实大部分都不会使用。第一个是版本号还是 socks5。第二个，就是返回的类型，这里是成功就返回0，第三个是保留字段填0，第四个 atype 地址类型填1，第五个，第六个暂时用不到，都填成 0。一共 4 + 4 + 2 个字节，后面6个字节都是 0 填充。

### relay 阶段
直接用 net.Dial 建立一个 TCP 连接，建立完连接之后，我们同样要加一个 defer 来关闭连接。接下来需要建立浏览器和下游服务器的双向数据转发，标准库的 io.copy 可以实现一个单向数据转发，双向转发的话，需要启动两个 go routinue。

现在有一个问题，connect 函数会立刻返回，返回的时候连接就被关闭了。需要等待任意一个方向 copy 出错的时候，再返回 connect 函数。这里可以使用到标准库里面的一个 context 机制，用 context 的 with cancel 来创建一个context，最后等待 ctx.Done，只要 cancel 被调用，ctx.Done 就会立刻返回。然后在上面的两个go routinue 里面调用一次 cancel 即可。

我们可以试着在浏览器里面测试一下，在浏览器里面测试代理需要安装 switchomega 插件，然后里面新建一个情景模式，代理服务器选 socks5，端口 1080，保存并启用。此时你应该还能够正常地访问网站代理服务器这边会显示出浏览器版本的域名和端口。
![Alt text](images/llq.png)