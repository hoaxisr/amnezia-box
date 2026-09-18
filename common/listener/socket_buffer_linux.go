package listener

import (
	"os"
	"strconv"
	"strings"
	"sync"

	C "github.com/sagernet/sing-box/constant"
)

// Апстрим просит C.UDPSocketBufferSize (8 МиБ) на КАЖДЫЙ UDP-сокет и ставит его
// через SO_RCVBUFFORCE/SO_SNDBUFFORCE (common/dialer/default.go,
// listener_udp.go). FORCE не клампится о net.core.rmem_max, поэтому под root
// один сокет получает sk_rcvbuf и sk_sndbuf по 16 МиБ (ядро удваивает значение).
//
// На Entware-роутере это ломает UDP целиком. Замер на Keenetic (254 МБ RAM,
// ядро 4.9-ndm-5): net.ipv4.udp_mem = 5886/7850/11772 страниц = 24/32/48 МБ —
// это ГЛОБАЛЬНЫЙ бюджет на все UDP-сокеты машины. Двух сокетов апстрим-размера
// хватает, чтобы его выбрать; дальше sk_rmem_schedule отказывает и
// __udp_enqueue_schedule_skb молча дропает дейтаграммы у всех сразу — включая
// DNS и транспортный сокет ядерного WireGuard. TCP при этом цел, потому что
// control.UDPSocketBuffer пропускает не-UDP сокеты. Симптом у пользователя —
// встающий QUIC при живом TCP, лечится баном UDP/443 (awg-manager#929).
//
// BSD-ветка соседнего файла уже считает размер от sysctl (kern.ipc.maxsockbuf).
// Linux — единственная платформа, где апстрим оставил константу. Здесь тот же
// приём: тот же запрос, но не больше доли от системного бюджета.
//
// СНЯТЬ ЦЕЛИКОМ, когда апстрим начнёт считать размер от sysctl и на Linux.
var UDPSocketBufferSize = sync.OnceValue(func() int {
	return clampUDPSocketBufferSize(C.UDPSocketBufferSize, udpMemMinPages(), os.Getpagesize())
})

// udpSocketBufferShare — сколько сокетов апстрим-размера должно помещаться в
// неприкосновенный резерв udp_mem[0], ниже которого ядро память не отбирает.
// Ядро удваивает значение setsockopt, поэтому делитель вдвое больше числа
// сокетов: 16 насыщенных сокетов упираются ровно в резерв.
const udpSocketBufferShare = 32

// clampUDPSocketBufferSize ограничивает запрошенный размер долей глобального
// резерва udp_mem[0]. На машине, которая может себе позволить запрошенное, это
// no-op: 16 ГБ RAM дают udp_mem[0] = 1453 МБ, доля = 45 МБ > 8 МиБ. На роутере
// с 254 МБ доля = 732 КиБ, и запрос урезается.
//
// Деление на долю идёт в СТРАНИЦАХ и только потом умножается на размер
// страницы — иначе на 32-битной арке (mipsel) большой udp_mem переполняет int.
func clampUDPSocketBufferSize(size, udpMemMin, pageSize int) int {
	if udpMemMin <= 0 || pageSize <= 0 {
		return size
	}
	share := udpMemMin / udpSocketBufferShare * pageSize
	if share > 0 && share < size {
		return share
	}
	return size
}

// udpMemMinPages читает первое поле net.ipv4.udp_mem (в страницах).
// 0 — прочитать не удалось, размер тогда не трогаем.
func udpMemMinPages() int {
	content, err := os.ReadFile("/proc/sys/net/ipv4/udp_mem")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0
	}
	return value
}
