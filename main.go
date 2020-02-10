package main

import (
  "io"
  "bufio"
  "fmt"
  "net"
  "os"
  "strings"
  "crypto/rand"
  "syscall"
  "time"
  
  log "github.com/sirupsen/logrus"
)

var devices = make(map[string]map[string]net.Conn)
var listening = 0;
var connections = make(map[string]net.Conn)
var active = 0;

// See https://stackoverflow.com/a/25736155
func pseudoUuid() (string) {
    b := make([]byte, 16)
    _, err := rand.Read(b)
    if err != nil {
        log.Panic(err)
    }
    return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

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
  cmd := readTrimmedLine(reader)
  address := readTrimmedLine(reader)
  clog = log.WithFields(log.Fields{
    "address": address,
    "cmd": cmd,
  })
  clog.Debug("command received")

  switch cmd {
    case "0":
      clog.Debug("adding device...")
      devices[address] = make(map[string]net.Conn)

    case "1":
      clog.Debug("removing device...")
      delete(devices, address)

    case "2":
      clog.Debug("sending devices...")
      writer := bufio.NewWriter(connection)
      for addr, services := range devices {
        if addr != address {
          entry := addr
          for uuid, _ := range services {
            entry += "," + uuid
          }
          entry += "\n"
          clog.Debug(entry)
          writer.WriteString(entry)
        }
      }
      writer.Flush()

    case "3":
          clog.Debug("adding service...")
          uuid := readTrimmedLine(reader)
          clog.Debug(uuid)

          devices[address][uuid] = connection
          defer func() {
            clog.Debug("removing service...")
            delete(devices[address], uuid)
          }()

          listening++
          defer func() {listening--}()

          clog.Debug("keeping listener connection...")
          for {
            if (connCheck(connection) != nil) {
              clog.Debug("listener connection closed");
              break;
            }
          }

    case "5":
          clog.Debug("adding client connection...")
          addr := readTrimmedLine(reader)
          clog.Debug(addr)
          uuid := readTrimmedLine(reader)
          clog.Debug(uuid)
          connId := pseudoUuid()
          clog.Debug(connId)

          connections[connId] = connection
          defer func() {
            clog.Debug("removing client connection...")
            delete(connections, connId)
          }()

          writer := bufio.NewWriter(devices[addr][uuid])
          writer.WriteString(address + "\n")
          writer.WriteString(connId + "\n")
          writer.Flush()

          clog.Debug("keeping client connection...")
          for {
            if (connCheck(connection) != nil) {
              clog.Debug("client connection closed");
              break;
            }
          }

    case "6":
          clog.Debug("linking client connection...")
          connId := readTrimmedLine(reader)
          clog.Debug(connId)

          clientConnection := connections[connId]
          defer func() {
            clog.Debug("closing client connection...")
            clientConnection.Close()
          }()

          active++
          defer func() {active--}()
          
          // There seem to be situations where neither of the following
          // functions ever return, even though the connections have been
          // closed on the emulator side. This should be investigated further.
          // However, the functions return at least, if the app that has created
          // the connection has been stopped. So it's not critical.

          writeDone := make(chan bool)

          go func() {
            defer func() { writeDone <- true }()
            clog.Debug("writing from client to server...")
            writer := bufio.NewWriter(connection)
            clientReader := bufio.NewReader(clientConnection)
            clientReader.WriteTo(writer)
            clog.Debug("writing from client to server done")
          }()

          go func() {
            defer func() { writeDone <- true }()
            clog.Debug("writing from server to client...")
            clientWriter := bufio.NewWriter(clientConnection)
            reader.WriteTo(clientWriter)
            clog.Debug("writing from server to client done")
          }()

          <-writeDone

    default:
      clog.Warn("unknown command...")
  }
}

func main() {
  if len(os.Args) >= 2 && os.Args[1] == "--debug" {
    log.SetLevel(log.DebugLevel)
  }

  ticker := time.NewTicker(5 * time.Minute)
  go func() {
    for {
        <- ticker.C
        log.WithFields(log.Fields{
          "devices": len(devices),
          "listening": listening,
          "connections": len(connections),
          "active": active,
        }).Info("statistics");
    }
   }()

  log.Info("starting service...")

  listener, err := net.Listen("tcp4", ":8199")
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
