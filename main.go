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
  
  log "github.com/sirupsen/logrus"
)

var CMDS = [...]string {"JOIN", "LEAVE", "DISCOVER", "ANNOUNCE", "LISTEN"}

type connSet struct {
  reader *bufio.Reader
  writer *bufio.Writer
  conn net.Conn
}

type groupSet struct {
  discoverable *map[string]connSet
  discovering *map[string]connSet
  services *map[string]map[string]connSet
  connections *map[string]connSet
}

var groups = make(map[string]groupSet)
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

func readTrimmedLine(reader *bufio.Reader) string {
    line, err := reader.ReadString('\n')
    if err != nil {
      log.Panic(err)
    }
    line = strings.TrimSpace(string(line))
    return line
}

func handleConnection(connection net.Conn) {
  var clog *log.Entry
  
  defer func() {
    clog.Debug("closing connection...")
    connection.Close()
  }()

  reader := bufio.NewReader(connection)
  group := readTrimmedLine(reader)
  cmd, _ := strconv.Atoi(readTrimmedLine(reader))
  address := readTrimmedLine(reader)
  
  println("Group: " + group + "; length: " + strconv.Itoa(len(group)))
  if len(group) == 0 {
    group = connection.RemoteAddr().String()
    group = group[:len(group)-6]
  }
 
  if _, exists := groups[group]; exists == false {
    log.Debug("create new lists for ", group)
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

  clog = log.WithFields(log.Fields{
    "address": address,
    "cmd": cmd,
    "group": group,
  })
  clog.Debug(CMDS[cmd] + " command received")

  switch cmd {
    case 0:
      clog.Debug("adding device...")
      writer := bufio.NewWriter(connection)
      discoverable[address] = connSet{reader, writer, connection}
      clog.Debug("keeping discoverable connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("discoverable connection closed.")
          clog.Debug("removing device...")
          delete(discoverable, address)
          break;
        }
      }

    case 1:
      clog.Warn("Usage of obsolete command")

    case 2:
      clog.Debug("register device as discovering...")
      writer := bufio.NewWriter(connection)
      discovering[address] = connSet{reader, writer, connection}
      clog.Debug("sending discovered services...")
      for addr, _ := range discoverable {
        if addr != address {
          entry := addr
          for uuid, _ := range services[addr] {
            entry += "," + uuid
          }
          entry += "\n"
          clog.Debug(entry)
          writer.WriteString(entry)
        }
      }
      writer.Flush()

      clog.Debug("keeping discovery connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("discovery connection closed")
          clog.Debug("unregister device...")
          delete(discovering, address)
          break;
        }
      }

    case 3:
      listening++
      defer func() {listening--}()

      clog.Debug("announcing service...")
      uuid := readTrimmedLine(reader)
      clog.Debug(uuid)
      
      writer := bufio.NewWriter(connection)

      if _, exists := services[address]; exists == false {
        services[address] = make(map[string]connSet)
      }
      services[address][uuid] = connSet{reader, writer, connection}
      defer func() {
        clog.Debug("removing service...")
        delete(services[address], uuid)
        if len(services[address]) == 0 {
          delete(services, address)
        }
      }()

      clog.Debug("keeping listener connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("listener connection closed")
          break;
        }
      }

    case 4:
      clog.Debug("adding client connection...")
      addr := readTrimmedLine(reader)
      clog.Debug(addr)
      uuid := readTrimmedLine(reader)
      clog.Debug(uuid)
      connId := getConnectionId()
      clog.Debug(connId)

      writer := bufio.NewWriter(connection)
      connections[connId] = connSet{reader, writer, connection}
      defer func() {
        clog.Debug("removing client connection...")
        delete(connections, connId)
      }()

      
      // TODO The following block should lock `services`
      if _, exists := services[addr]; exists == false {
        clog.Info("address of service does not exist");
        return
      }
      if _, exists := services[addr][uuid]; exists == false {
        clog.Info("uuid of service does not exist");
        return
      }
      listenWriter := services[addr][uuid].writer
      listenWriter.WriteString(address + "\n")
      listenWriter.WriteString(connId + "\n")
      listenWriter.Flush()

      clog.Debug("keeping client connection...")
      for {
        if (connCheck(connection) != nil) {
          clog.Debug("client connection closed")
          break;
        }
      }

    case 5:
      active++
      defer func() {active--}()
      
      clog.Debug("linking client connection...")
      connId := readTrimmedLine(reader)
      clog.Debug(connId)

      // TODO The following block should lock `connections`
      if _, exists := connections[connId]; exists == false {
        clog.Info("connection does not exist");
        return
      }
      clientConnection := connections[connId].conn
      clientReader := connections[connId].reader
      clientWriter := connections[connId].writer
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
