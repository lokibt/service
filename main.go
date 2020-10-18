package main

import (
  "bufio"
  "io"
  "net"
  "os"
  "strings"
  "strconv"
  "syscall"
  "time"
  "sync"
  
  log "github.com/sirupsen/logrus"
)

var CMDS = [...]string {"JOIN", "LEAVE", "DISCOVERY", "LISTEN", "CONNECT", "LINK"}
const CONN_TIMEOUT = 10

type connSet struct {
  reader *bufio.Reader
  writer *bufio.Writer
  conn net.Conn
  available bool
}

type groupSet struct {
  discoverable *map[string]connSet
  discovering *map[string]connSet
  serving *map[string]map[string]connSet
  connections *map[string]*connSet
}

var groups = make(map[string]groupSet)
var groupsM sync.Mutex
var active = 0
var nextConnId = 0

// See https://stackoverflow.com/a/58664631
func connCheck(conn net.Conn) error {
    var sysErr error = nil
    rc, err := conn.(syscall.Conn).SyscallConn()
    if err != nil {
      return err
    }
    err = rc.Read(func(fd uintptr) bool {
        var buf []byte = []byte{0}
        n, _, err := syscall.Recvfrom(int(fd), buf, syscall.MSG_PEEK | syscall.MSG_DONTWAIT)
        switch {
          case n == 0 && err == nil:
            sysErr = io.EOF
          case err == syscall.EAGAIN || err == syscall.EWOULDBLOCK:
            sysErr = nil
          default:
            sysErr = err
        }
        return true
    })
    if err != nil {
      return err
    }
    return sysErr
}

func getConnectionId() (string) {
  nextConnId++
  return strconv.Itoa(nextConnId)
}


func handleConnection(connection net.Conn) {
  active++
  defer func() {active--}()
  
  clog := log.WithFields(log.Fields{})
  
  defer func() {
    clog.Debug("closing connection ", connection.RemoteAddr())
    connection.Close()
    if r := recover(); r != nil {
      clog.Debug("recovered from panic")
    }
  }()

  readTrimmedLine := func(reader *bufio.Reader) string {
      line, err := reader.ReadString('\n')
      if err != nil {
        clog.Panic(err)
      }
      line = strings.TrimSpace(string(line))
      return line
  }

  reader := bufio.NewReader(connection)
  group := readTrimmedLine(reader)
  cmd, _ := strconv.Atoi(readTrimmedLine(reader))
  address := readTrimmedLine(reader)
  
  if len(group) == 0 {
    clog.Debug("Group not defined, using IP address")
    group = connection.RemoteAddr().String()
    group = group[:len(group)-6]
  }


  clog = log.WithFields(log.Fields{
    "address": address,
    "cmd": cmd,
    "group": group,
  })
  clog.Debug(CMDS[cmd] + " command received")
 
  groupsM.Lock();
  if _, exists := groups[group]; exists == false {
    clog.Debug("create new lists for ", group)
    dis := make(map[string]connSet)
    dbl := make(map[string]connSet)
    ser := make(map[string]map[string]connSet)
    con := make(map[string]*connSet)
    groups[group] = groupSet { &dis, &dbl, &ser, &con }
  }
  discoverable := *groups[group].discoverable
  discovering := *groups[group].discovering
  serving := *groups[group].serving
  connections := *groups[group].connections
  groupsM.Unlock();

  defer func() {
    groupsM.Lock();
    if len(discoverable) + len(discovering) + len(serving) + len(connections) == 0 {
      clog.Debug("Removing empty group...")
      delete(groups, group)
    }
    groupsM.Unlock();
  }()

  switch cmd {
    case 0: // JOIN
      writer := bufio.NewWriter(connection)

      clog.Debug("adding device...")
      groupsM.Lock();
      discoverable[address] = connSet{reader, writer, connection, true}
      groupsM.Unlock();

      defer func() {
          clog.Debug("removing device...")
          groupsM.Lock();
          delete(discoverable, address)
          groupsM.Unlock();
      }()

      clog.Debug("notify discovering devices...")
      for addr, other := range discovering {
        clog.Debug(addr + " will be notified")
        other.writer.WriteString(address + "\n")
        other.writer.Flush()
      }

      clog.Debug("keeping discoverable connection ", connection.RemoteAddr())
      for (connCheck(connection) == nil) { }
      clog.Debug("discoverable connection ", connection.RemoteAddr(), " closed")

    case 1: // LEAVE
      clog.Warn("Usage of obsolete command")

    case 2: // DISCOVER
      writer := bufio.NewWriter(connection)

      clog.Debug("register device as discovering...")
      groupsM.Lock();
      discovering[address] = connSet{reader, writer, connection, true}
      groupsM.Unlock();

      defer func() {
          clog.Debug("unregister device as discovering...")
          groupsM.Lock();
          delete(discovering, address)
          groupsM.Unlock();
      }()

      clog.Debug("sending discoverable devices...")
      for addr, _ := range discoverable {
        if addr != address {
          clog.Debug(addr + " will be sent")
          writer.WriteString(addr + "\n")
        }
      }
      writer.Flush()

      clog.Debug("keeping discovery connection ", connection.RemoteAddr())
      for (connCheck(connection) == nil) { }
      clog.Debug("discovery connection ", connection.RemoteAddr(), " closed")

    case 3: // LISTEN
      uuid := readTrimmedLine(reader)
      clog.Debug(uuid)
      
      writer := bufio.NewWriter(connection)

      groupsM.Lock();
      if _, exists := serving[address]; exists == false {
        serving[address] = make(map[string]connSet)
      }
      if _, exists := serving[address][uuid]; exists == true {
        clog.Debug("service ", uuid, " is already listening");
        groupsM.Unlock();
        return;
      }
      clog.Debug("add service...")
      serving[address][uuid] = connSet{reader, writer, connection, true}
      groupsM.Unlock();

      defer func() {
        clog.Debug("remove service...")
        groupsM.Lock();
        delete(serving[address], uuid)
        if len(serving[address]) == 0 {
          delete(serving, address)
        }
        groupsM.Unlock();
      }()

      clog.Debug("keeping listen connection ", connection.RemoteAddr())
      for (connCheck(connection) == nil) { }
      clog.Debug("listen connection ", connection.RemoteAddr(), " closed")

    case 4: // CONNECT
      addr := readTrimmedLine(reader)
      clog.Debug(addr)
      uuid := readTrimmedLine(reader)
      clog.Debug(uuid)
      connId := getConnectionId()
      clog.Debug(connId)

      writer := bufio.NewWriter(connection)
      clog.Debug("adding client connection...")
      groupsM.Lock();
      connections[connId] = &connSet{reader, writer, connection, true}
      groupsM.Unlock();

      defer func() {
        clog.Debug("removing client connection...")
        groupsM.Lock();
        delete(connections, connId)
        groupsM.Unlock();
      }()
      
      groupsM.Lock();
      if _, exists := serving[addr]; exists == false {
        clog.Info("address of service does not exist")
        writer.WriteString("fail\n")
        writer.Flush()
        groupsM.Unlock();
        return
      }
      if _, exists := serving[addr][uuid]; exists == false {
        clog.Info("uuid of service does not exist")
        writer.WriteString("fail\n")
        writer.Flush()
        groupsM.Unlock();
        return
      }
      if serving[addr][uuid].available == false {
        clog.Info("service is already in use")
        writer.WriteString("fail\n")
        writer.Flush()
        groupsM.Unlock();
        return
      }
      service := serving[addr][uuid]
      service.available = false
      serving[addr][uuid] = service;
      clog.Debug("service has been marked as in use")
      groupsM.Unlock();

      defer func() {
        groupsM.Lock();
        if _, exists := serving[addr][uuid]; exists == true {
          service := serving[addr][uuid]
          service.available = true
          serving[addr][uuid] = service;
          clog.Debug("service is not in use anymore")
        }
        groupsM.Unlock();
      }()

      listenWriter := serving[addr][uuid].writer
      listenWriter.WriteString(address + "\n")
      listenWriter.WriteString(connId + "\n")
      listenWriter.Flush()

      writer.WriteString("ok\n")
      writer.Flush()
      clog.Debug("keeping client connection (", CONN_TIMEOUT , "s timeout) ", connection.RemoteAddr())
      // connCheck() might never retun an error, if some data have already been sent by the client, but the server would
      // never respond with a LINK command. So we have to add a timeout to ensure that the connection is closed, if a
      // LINK command for the connection has not been received within CONN_TIMEOUT seconds.
      timeout := time.Now().Unix() + CONN_TIMEOUT;
      for (connCheck(connection) == nil) {
        if (connections[connId].available && (time.Now().Unix() >= timeout)) {
          clog.Debug("client connection ", connection.RemoteAddr(), " timed out")
          return
        }
      }
      clog.Debug("client connection ", connection.RemoteAddr(), " closed")

    case 5: // LINK
      clog.Debug("linking client connection...")
      connId := readTrimmedLine(reader)
      clog.Debug(connId)

      groupsM.Lock();
      if _, exists := connections[connId]; exists == false {
        clog.Info("connection does not exist");
        groupsM.Unlock();
        return
      }
      clientConnection := connections[connId].conn
      clientReader := connections[connId].reader
      clientWriter := connections[connId].writer
      connections[connId].available = false;
      groupsM.Unlock();

      defer func() {
        clog.Debug("closing client connection ", clientConnection.RemoteAddr())
        clientConnection.Close()
      }()

      writeDone := make(chan bool)

      go func() {
        defer func() { writeDone <- true }()
        clog.Debug("writing from client to server...")
        writer := bufio.NewWriter(connection)
        io.Copy(writer, clientReader)
        clog.Debug("writing from client to server done")
      }()

      go func() {
        defer func() { writeDone <- true }()
        clog.Debug("writing from server to client...")
        io.Copy(clientWriter, reader)
        clog.Debug("writing from server to client done")
      }()

      <- writeDone

    default:
      clog.Warn("unknown command...")
  }
}

func compileStats() (int, int, int, int, int) {
  discoverable := 0
  discovering := 0
  serving := 0
  connections := 0
  for group := range groups {
    discoverable += len(*groups[group].discoverable)
    discovering += len(*groups[group].discovering)
    serving += len(*groups[group].serving)
    connections += len(*groups[group].connections)
  }
  return len(groups), discoverable, discovering, serving, connections
}

func logStats() {
  ticker := time.NewTicker(1 * time.Minute)
  for {
    groups, discoverable, discovering, serving, connections := compileStats()
    log.WithFields(log.Fields{
      "active": active,
      "groups": groups,
      "discoverable": discoverable,
      "discovering": discovering,
      "serving": serving,
      "connections": connections,
    }).Info("statistics");
    <- ticker.C
  }
}

func main() {
  log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
  if len(os.Args) >= 2 && os.Args[1] == "--debug" {
    log.SetLevel(log.DebugLevel)
  }
  
  log.Info("starting service...")

  go logStats()

  listener, err := net.Listen("tcp4", ":8198")
  if err != nil {
    log.Fatal(err)
    return
  }

  defer func() {
    log.Info("stopping service...")
    listener.Close()
  }()

  for {
    connection, err := listener.Accept()
    if err != nil {
      log.Error(err)
      return
    }
    go handleConnection(connection)
  }
}
