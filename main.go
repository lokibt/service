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

var CMDS = [...]string {"JOIN", "LEAVE", "DISCOVERY", "ANNOUNCE", "CONNECT", "LINK"}

type connSet struct {
  reader *bufio.Reader
  writer *bufio.Writer
  conn net.Conn
  available bool
}

type groupSet struct {
  discoverable *map[string]connSet
  discovering *map[string]connSet
  services *map[string]map[string]connSet
  connections *map[string]connSet
}

var groups = make(map[string]groupSet)
var groupsM sync.Mutex
var listening = 0;
var active = 0;
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
  clog := log.WithFields(log.Fields{})
  
  defer func() {
    clog.Debug("closing connection...")
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
 
  if _, exists := groups[group]; exists == false {
    clog.Debug("create new lists for ", group)
    dis := make(map[string]connSet)
    dbl := make(map[string]connSet)
    ser := make(map[string]map[string]connSet)
    con := make(map[string]connSet)
    groups[group] = groupSet { &dis, &dbl, &ser, &con }
  }
  discoverable := *groups[group].discoverable
  discovering := *groups[group].discovering
  services := *groups[group].services
  connections := *groups[group].connections

  switch cmd {
    case 0: // JOIN
      clog.Debug("adding device...")
      writer := bufio.NewWriter(connection)
      groupsM.Lock();
      discoverable[address] = connSet{reader, writer, connection, true}
      groupsM.Unlock();
      
      clog.Debug("notify discovering devices...")
      for addr, other := range discovering {
        clog.Debug(addr + " will be notified")
        other.writer.WriteString(address + "\n")
        other.writer.Flush()
      }
      
      clog.Debug("keeping discoverable connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("discoverable connection closed.")
          clog.Debug("removing device...")
          groupsM.Lock();
          delete(discoverable, address)
          groupsM.Unlock();
          break;
        }
      }

    case 1: // LEAVE
      clog.Warn("Usage of obsolete command")

    case 2: // DISCOVER
      clog.Debug("register device as discovering...")
      writer := bufio.NewWriter(connection)
      groupsM.Lock();
      discovering[address] = connSet{reader, writer, connection, true}
      groupsM.Unlock();

      clog.Debug("sending discoverable devices...")
      for addr, _ := range discoverable {
        if addr != address {
          clog.Debug(addr + " will be sent")
          writer.WriteString(addr + "\n")
        }
      }
      writer.Flush()

      clog.Debug("keeping discovery connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("discovery connection closed")
          clog.Debug("unregister device...")
          groupsM.Lock();
          delete(discovering, address)
          groupsM.Unlock();
          break;
        }
      }

    case 3: // ANNOUNCE
      listening++
      defer func() {listening--}()

      clog.Debug("announcing service...")
      uuid := readTrimmedLine(reader)
      clog.Debug(uuid)
      
      writer := bufio.NewWriter(connection)

      groupsM.Lock();
      if _, exists := services[address]; exists == false {
        services[address] = make(map[string]connSet)
      }
      if _, exists := services[address][uuid]; exists == true {
        clog.Debug("service has already been announced");
        groupsM.Unlock();
        return;
      }
      services[address][uuid] = connSet{reader, writer, connection, true}
      groupsM.Unlock();

      defer func() {
        groupsM.Lock();
        clog.Debug("removing service...")
        delete(services[address], uuid)
        if len(services[address]) == 0 {
          delete(services, address)
        }
        groupsM.Unlock();
      }()

      clog.Debug("keeping listener connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("listener connection closed")
          break;
        }
      }

    case 4: // CONNECT
      clog.Debug("adding client connection...")
      addr := readTrimmedLine(reader)
      clog.Debug(addr)
      uuid := readTrimmedLine(reader)
      clog.Debug(uuid)
      connId := getConnectionId()
      clog.Debug(connId)

      writer := bufio.NewWriter(connection)
      groupsM.Lock();
      connections[connId] = connSet{reader, writer, connection, true}
      groupsM.Unlock();

      defer func() {
        clog.Debug("removing client connection...")
        groupsM.Lock();
        delete(connections, connId)
        groupsM.Unlock();
      }()
      
      groupsM.Lock();
      if _, exists := services[addr]; exists == false {
        clog.Info("address of service does not exist")
        writer.WriteString("fail\n")
        writer.Flush()
        groupsM.Unlock();
        return
      }
      if _, exists := services[addr][uuid]; exists == false {
        clog.Info("uuid of service does not exist")
        writer.WriteString("fail\n")
        writer.Flush()
        groupsM.Unlock();
        return
      }
      if services[addr][uuid].available == false {
        clog.Info("service is already in use")
        writer.WriteString("fail\n")
        writer.Flush()
        groupsM.Unlock();
        return
      }
      service := services[addr][uuid]
      service.available = false
      services[addr][uuid] = service;
      clog.Debug("service has been marked as in use")
      groupsM.Unlock();

      defer func() {
        groupsM.Lock();
        if _, exists := services[addr][uuid]; exists == true {
          service := services[addr][uuid]
          service.available = true
          services[addr][uuid] = service;
          clog.Debug("service is not in use anymore")
        }
        groupsM.Unlock();
      }()

      listenWriter := services[addr][uuid].writer
      listenWriter.WriteString(address + "\n")
      listenWriter.WriteString(connId + "\n")
      listenWriter.Flush()

      writer.WriteString("ok\n")
      writer.Flush()
      clog.Debug("keeping client connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("client connection closed")
          break;
        }
      }

    case 5: // LINK
      active++
      defer func() {active--}()
      
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
      groupsM.Unlock();

      defer func() {
        clog.Debug("closing client connection...")
        clientConnection.Close()
      }()

      writeDone := make(chan bool)

      go func() {
        defer func() { writeDone <- true }()
        clog.Debug("writing from client to server...")
        writer := bufio.NewWriter(connection)
        // TODO Investigate why clientReader.WriteTo(writer) does not work here
        writer.ReadFrom(clientReader)
        // Use the following code for debugging
        /*for {
          line, err := clientReader.ReadString('\n')
          if (err != nil) {
            if (err != io.EOF) {
              log.Error("=> ", "read error ", err)
              break;
            }
          } else {
            writer.WriteString(line)
            if (err != nil) {
              log.Error("=> ", "write error: ", err)
              break
            }
            writer.Flush()
            if (err != nil) {
              log.Error("=> ", "flush error: ", err)
              break;
            }
            log.Debug("=> ", line)
          }
        }//*/
        clog.Debug("writing from client to server done")
      }()

      go func() {
        defer func() { writeDone <- true }()
        clog.Debug("writing from server to client...")
        // TODO Investigate why reader.WriteTo(clientWriter) does not work here
        clientWriter.ReadFrom(reader)
        // Use the following code for debugging
        /*for {
          line, err := reader.ReadString('\n')
          if (err != nil) {
            if (err != io.EOF) {
              log.Error("<= ", "read error: ", err)
              break
            }
          } else {
            _, err = clientWriter.WriteString(line)
            if (err != nil) {
              log.Error("<= ", "write error: ", err)
              break
            }
            clientWriter.Flush()
            if (err != nil) {
              log.Error("<= ", "flush error: ", err)
              break;
            }
            log.Debug("<= ", line)
          }
        }*/
        clog.Debug("writing from server to client done")
      }()

      <- writeDone

    default:
      clog.Warn("unknown command...")
  }
}

func main() {
  log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
  if len(os.Args) >= 2 && os.Args[1] == "--debug" {
    log.SetLevel(log.DebugLevel)
  }

  ticker := time.NewTicker(5 * time.Minute)
  go func() {
    for {
      <- ticker.C
      log.WithFields(log.Fields{
        "groups": len(groups),
        "listening": listening,
        "active": active,
      }).Info("statistics");
    }
   }()

  log.Info("starting service...")

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
