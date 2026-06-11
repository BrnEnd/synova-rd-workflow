package evolution

import (
	"strings"
	"testing"
	"time"

	adminSvc "synova-rd-workflow/internal/service/admin"
)

func TestCalculateTaskTemporalStatus(t *testing.T) {
	loc := saoPauloLocation()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, loc)

	cases := []struct {
		name string
		date time.Time
		want taskTemporalStatus
	}{
		{name: "overdue", date: time.Date(2026, 6, 9, 12, 22, 0, 0, loc), want: taskStatusOverdue},
		{name: "today", date: time.Date(2026, 6, 11, 16, 0, 0, 0, loc), want: taskStatusToday},
		{name: "tomorrow", date: time.Date(2026, 6, 12, 9, 57, 0, 0, loc), want: taskStatusTomorrow},
		{name: "this week", date: time.Date(2026, 6, 14, 9, 0, 0, 0, loc), want: taskStatusThisWeek},
		{name: "next week", date: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), want: taskStatusNextWeek},
		{name: "future", date: time.Date(2026, 7, 1, 9, 0, 0, 0, loc), want: taskStatusFuture},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateTaskTemporalStatus(tc.date, now); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestSanitizeTaskText_RemovesLocalPaths(t *testing.T) {
	input := `PHARMA CHEMICAL - file:///C:/Users/Marco/Downloads/audio.ogg MAXSIL F200 C:\Users\Marco\Downloads\outro.txt`
	got := sanitizeTaskText(input)
	if strings.Contains(got, "file:///") || strings.Contains(got, `C:\Users`) {
		t.Fatalf("expected local paths removed, got %q", got)
	}
	if got != "PHARMA CHEMICAL - MAXSIL F200" {
		t.Fatalf("unexpected sanitized text: %q", got)
	}
}

func TestFormatScheduledTasks_RecalculatesStatusSanitizesAndLimits(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, saoPauloLocation())
	summary := adminSvc.ScheduledTaskSummary{
		Name:   "Tarefas pendentes do RD Station",
		Active: true,
		PendingRDTasks: []adminSvc.ScheduledRDPendingTask{
			{
				Subject:          "Futura",
				Date:             "2026-07-01",
				Hour:             "09:00",
				Markup:           "today",
				DealName:         "Z Futuro",
				ResponsibleNames: []string{"Glauco"},
			},
			{
				Subject:          "Atrasada",
				Date:             "2026-06-09",
				Hour:             "12:22",
				Markup:           "tomorrow",
				DealName:         "Hubio Agro file:///C:/Users/Marco/Downloads/audio.ogg",
				ResponsibleNames: []string{"Guilherme"},
				Notes:            "Entrar em contato C:\\Users\\Marco\\Downloads\\nota.txt com prioridade.",
			},
			{
				Subject:          "Sem horario",
				Date:             "2026-06-11",
				Markup:           "week-0",
				DealName:         "Hoje",
				ResponsibleNames: []string{"Glauco"},
			},
			{
				Subject:  "Amanha",
				Date:     "2026-06-12",
				Hour:     "09:57",
				Markup:   "week-1",
				DealName: "Zomix",
			},
		},
	}

	got := formatRDTaskPage(collectFormattedRDTasks([]adminSvc.ScheduledTaskSummary{summary}, now), 0, 10)

	for _, forbidden := range []string{"tomorrow", "week-0", "week-1", "file:///", `C:\Users`, "Regra:", "Etapa monitorada"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("response contains forbidden text %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "Status: Atrasada") {
		t.Fatalf("expected recalculated overdue status:\n%s", got)
	}
	if !strings.Contains(got, "Status: Para hoje") {
		t.Fatalf("expected today status:\n%s", got)
	}
	if !strings.Contains(got, "Status: Amanha") {
		t.Fatalf("expected tomorrow status:\n%s", got)
	}
	if !strings.Contains(got, "Status: Futura") {
		t.Fatalf("expected future status:\n%s", got)
	}
	if !strings.Contains(got, "Data: 11/06/2026\nStatus: Para hoje") {
		t.Fatalf("expected date without invented hour:\n%s", got)
	}
	if strings.Index(got, "*Atrasada*") > strings.Index(got, "*Sem horario*") {
		t.Fatalf("expected overdue task before today's task:\n%s", got)
	}
}

func TestFormatRDTaskPage_PaginatesAtTenItems(t *testing.T) {
	tasks := make([]formattedRDTask, 11)
	for i := range tasks {
		tasks[i] = formattedRDTask{
			Subject:  "Tarefa",
			DateText: "11/06/2026",
			Status:   taskStatusToday,
		}
	}

	got := formatRDTaskPage(tasks, 0, rdTaskPageSize)
	if !strings.Contains(got, "Exibindo 10 de 11 tarefas") {
		t.Fatalf("expected pagination hint, got:\n%s", got)
	}
	if strings.Count(got, ". *Tarefa*") != 10 {
		t.Fatalf("expected exactly 10 tasks, got:\n%s", got)
	}
}
