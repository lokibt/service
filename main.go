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
  
  log "github.com/sirupsen/logrus"
)

var devices = make(map[string]map[string]net.Conn)
var connections = make(map[string]net.Conn)

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
    clog.Info("closing connection...")
    connection.Close()
  }()

  reader := bufio.NewReader(connection)
  cmd := readTrimmedLine(reader)
  address := readTrimmedLine(reader)
  clog = log.WithFields(log.Fields{
    "address": address,
    "cmd": cmd,
  })
  clog.Info("command received")

  switch cmd {
    case "0":
      clog.Info("adding device...")
      devices[address] = make(map[string]net.Conn)

    case "1":
      clog.Info("removing device...")
      delete(devices, address)

    case "2":
      clog.Info("sending devices...")
      writer := bufio.NewWriter(connection)
      for addr, services := range devices {
        if addr != address {
          entry := addr
          for uuid, _ := range services {
            entry += "," + uuid
          }
          entry += "\n"
          log.Debug(entry)
          writer.WriteString(entry)
        }
      }
      writer.Flush()

    case "3":
          clog.Info("adding service...")
          uuid := readTrimmedLine(reader)
          log.Debug(uuid)

          devices[address][uuid] = connection
          defer func() {
            clog.Info("removing service...")
            delete(devices[address], uuid)
          }()

          clog.Info("keeping listener connection...")
          for {
            if (connCheck(connection) != nil) {
              clog.Info("listener connection closed");
              break;
            }
          }

    case "5":
          clog.Info("adding client connection...")
          addr := readTrimmedLine(reader)
          log.Debug(addr)
          uuid := readTrimmedLine(reader)
          log.Debug(uuid)
          connId := pseudoUuid()
          log.Debug(connId)

          connections[connId] = connection
          defer func() {
            clog.Info("removing client connection...")
            delete(connections, connId)
          }()

          writer := bufio.NewWriter(devices[addr][uuid])
          writer.WriteString(address + "\n")
          writer.WriteString(connId + "\n")
          writer.Flush()

          clog.Info("keeping client connection...")
          for {
            if (connCheck(connection) != nil) {
              clog.Info("client connection closed");
              break;
            }
          }

    case "6":
          clog.Info("linking client connection...")
          connId := readTrimmedLine(reader)
          log.Debug(connId)

          clientConnection := connections[connId]
          defer func() {
            clog.Info("closing client connection...")
            clientConnection.Close()
          }()
          
          // There seem to be situations where neither of the following
          // functions ever return, even though the connections have been
          // closed on the emulator side. This should be investigated further.
          // However, the functions return at least, if the app that has created
          // the connection has been stopped. So it's not critical.

          writeDone := make(chan bool)

          go func() {
            defer func() { writeDone <- true }()
            clog.Info("writing from client to server...")
            writer := bufio.NewWriter(connection)
            clientReader := bufio.NewReader(clientConnection)
            clientReader.WriteTo(writer)
            clog.Info("writing from client to server done")
          }()

          go func() {
            defer func() { writeDone <- true }()
            clog.Info("writing from server to client...")
            clientWriter := bufio.NewWriter(clientConnection)
            reader.WriteTo(clientWriter)
            clog.Info("writing from server to client done")
          }()

          <-writeDone

    default:
      clog.warn("unknown command...")
  }
}

func main() {
  if len(os.Args) == 1 {
    fmt.Println("Please provide a port number!")
    return
  }
  port := ":" + os.Args[1]

  listener, err := net.Listen("tcp4", port)
  if err != nil {
    log.Info(err)
    return
  }

  defer func() {
    log.Info("stop listening...")
    listener.Close()
  }()

  log.Info("start listening...")
  for {
    connection, err := listener.Accept()
    if err != nil {
      log.Info(err)
      return
    }
    go handleConnection(connection)
  }
}
