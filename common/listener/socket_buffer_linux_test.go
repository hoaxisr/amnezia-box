package listener

import "testing"

// Цифры взяты с живых машин, а не придуманы: роутер — Keenetic 254 МБ RAM,
// ядро 4.9-ndm-5 (udp_mem 5886/7850/11772); десктоп — 16 ГБ RAM.
func TestClampUDPSocketBufferSize(t *testing.T) {
	const upstream = 8 << 20
	for _, tc := range []struct {
		name      string
		udpMemMin int
		pageSize  int
		want      int
	}{
		// 5886/32 = 183 страницы = 749568 Б. Запрос апстрима урезается почти
		// на порядок: 16 таких сокетов = udp_mem[0], а не 2 сокета на 48 МБ.
		{"keenetic 254MB", 5886, 4096, 749568},
		// 371949/32 = 11623 страницы = 47.6 МБ > 8 МиБ — запрос проходит целиком.
		{"desktop 16GB", 371949, 4096, upstream},
		// sysctl не прочитался — не угадываем, отдаём запрошенное.
		{"unreadable", 0, 4096, upstream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampUDPSocketBufferSize(upstream, tc.udpMemMin, tc.pageSize); got != tc.want {
				t.Errorf("clampUDPSocketBufferSize(%d, %d, %d) = %d, want %d",
					upstream, tc.udpMemMin, tc.pageSize, got, tc.want)
			}
		})
	}
}

// Сторож переполнения: на mipsel/386 int 32-битный. Число страниц подобрано
// так, что умножение ДО деления переполняет int32 и заворачивается в малое
// положительное — то есть даёт молчаливый неверный кламп, а не отказ.
// 1100000 страниц = 4.3 ГБ udp_mem[0], это машина на ~48 ГБ RAM.
func TestClampUDPSocketBufferSize_NoOverflowOn32Bit(t *testing.T) {
	const upstream = 8 << 20
	got := clampUDPSocketBufferSize(upstream, 1100000, 4096)
	if got != upstream {
		t.Fatalf("got %d, want %d: бюджет 4.3 ГБ, урезать нечего — значит переполнение", got, upstream)
	}
}

// Кламп обязан быть привязан к udp_mem, а не к константе: вдвое меньший
// бюджет обязан дать вдвое меньший буфер.
func TestClampUDPSocketBufferSize_ScalesWithBudget(t *testing.T) {
	const upstream = 8 << 20
	big := clampUDPSocketBufferSize(upstream, 5886, 4096)
	small := clampUDPSocketBufferSize(upstream, 2943, 4096)
	if small >= big {
		t.Fatalf("бюджет вдвое меньше, а буфер не уменьшился: %d >= %d", small, big)
	}
}
