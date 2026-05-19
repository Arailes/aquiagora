package main

import (
	"log"
	"math"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
)

// Estrutura para gerenciar o estado da chamada e a lista de alunos em memória
type EstadoChamada struct {
	sync.RWMutex
	Ativa           bool     `json:"ativa"`
	ProfLat         float64  `json:"prof_lat"`
	ProfLng         float64  `json:"prof_lng"`
	ProfIpPrefixo   string   `json:"prof_ip_prefixo"`
	Materia         string   `json:"materia"`
	AlunosPresentes []string `json:"alunos_presentes"`
}

var chamadaAtual EstadoChamada

type IniciarChamadaPayload struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Materia   string  `json:"materia"`
}

type ResponderChamadaPayload struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy"`
	AlunoNome string  `json:"aluno_nome"`
}

func extrairPrefixoIP(ip string) string {
	if strings.Contains(ip, "]") {
		ip = strings.Split(ip, "]")[0]
		ip = strings.Replace(ip, "[", "", 1)
	} else if strings.Count(ip, ":") <= 1 && strings.Contains(ip, ":") {
		ip = strings.Split(ip, ":")[0]
	}

	if strings.Contains(ip, ":") {
		blocos := strings.Split(ip, ":")
		if len(blocos) >= 4 {
			return strings.Join(blocos[:4], ":")
		}
		return ip
	}
	return ip
}

func obterIPReal(c *fiber.Ctx) string {
	xForwardedFor := c.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}
	return c.IP()
}

func calcularHaversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000
	rad := math.Pi / 180

	deltaPhi := (lat2 - lat1) * rad
	deltaLambda := (lon2 - lon1) * rad

	phi1 := lat1 * rad
	phi2 := lat2 * rad

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*
			math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func main() {
	app := fiber.New(fiber.Config{
		AppName: "Fluxsus - Chamada Inteligente Go v1.2",
	})

	app.Post("/api/abrir", func(c *fiber.Ctx) error {
		var payload IniciarChamadaPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(400).JSON(fiber.Map{"detail": "Payload inválido."})
		}

		ipReal := obterIPReal(c)

		chamadaAtual.Lock()
		chamadaAtual.Ativa = true
		chamadaAtual.ProfLat = payload.Latitude
		chamadaAtual.ProfLng = payload.Longitude
		chamadaAtual.ProfIpPrefixo = extrairPrefixoIP(ipReal)
		chamadaAtual.Materia = payload.Materia
		chamadaAtual.AlunosPresentes = []string{}
		chamadaAtual.Unlock()

		return c.JSON(fiber.Map{
			"status":              "sucesso",
			"mensagem":            "Chamada de " + payload.Materia + " iniciada com sucesso!",
			"prefixo_rede_ancora": chamadaAtual.ProfIpPrefixo,
		})
	})

	app.Post("/api/responder", func(c *fiber.Ctx) error {
		chamadaAtual.RLock()
		ativa := chamadaAtual.Ativa
		profLat := chamadaAtual.ProfLat
		profLng := chamadaAtual.ProfLng
		profPrefixo := chamadaAtual.ProfIpPrefixo
		chamadaAtual.RUnlock()

		if !ativa {
			return c.Status(400).JSON(fiber.Map{"detail": "Nenhuma chamada está ativa no momento."})
		}

		var payload ResponderChamadaPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(400).JSON(fiber.Map{"detail": "Dados de envio incorretos."})
		}

		ipAluno := obterIPReal(c)
		prefixoAluno := extrairPrefixoIP(ipAluno)

		if prefixoAluno != profPrefixo {
			return c.Status(403).JSON(fiber.Map{
				"detail": "Bloqueado: Você precisa estar conectado ao mesmo Wi-Fi do professor.",
			})
		}

		distancia := calcularHaversine(profLat, profLng, payload.Latitude, payload.Longitude)
		margemErroAceita := math.Min(payload.Accuracy, 20.0)
		raioMaximoPermitido := 15.0 + margemErroAceita

		if distancia > raioMaximoPermitido {
			return c.Status(400).JSON(fiber.Map{
				"detail": "Fora de Alcance: Você está muito afastado do professor para validar a presença.",
			})
		}

		chamadaAtual.Lock()
		chamadaAtual.AlunosPresentes = append(chamadaAtual.AlunosPresentes, payload.AlunoNome)
		chamadaAtual.Unlock()

		return c.JSON(fiber.Map{
			"status":   "sucesso",
			"mensagem": "Presença confirmada para " + payload.AlunoNome + "!",
		})
	})

	app.Get("/api/status", func(c *fiber.Ctx) error {
		chamadaAtual.RLock()
		defer chamadaAtual.RUnlock()
		return c.JSON(fiber.Map{
			"ativa":   chamadaAtual.Ativa,
			"materia": chamadaAtual.Materia,
		})
	})

	app.Post("/api/fechar", func(c *fiber.Ctx) error {
		chamadaAtual.Lock()
		alunos := chamadaAtual.AlunosPresentes

		chamadaAtual.Ativa = false
		chamadaAtual.AlunosPresentes = []string{}
		chamadaAtual.Unlock()

		return c.JSON(fiber.Map{
			"status":   "sucesso",
			"mensagem": "Chamada encerrada. Gerando relatório...",
			"alunos":   alunos,
		})
	})

	// Nova sintaxe do pacote estático embutido do Fiber
	app.Static("/", "./static")

	log.Println("Servidor rodando na porta 8000...")
	log.Fatal(app.Listen(":8000"))
}
