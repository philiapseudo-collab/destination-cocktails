package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/core"
	"github.com/jung-kurt/gofpdf"
)

const (
	reportTimezoneName      = "Africa/Nairobi"
	businessDayStartHourEAT = 7
)

var settledSalesStatuses = []core.OrderStatus{
	core.OrderStatusPaid,
	core.OrderStatusReady,
	core.OrderStatusCompleted,
}

// GenerateDailySalesReportPDF generates a PDF report for one operational business day.
// Business day window: 07:00 EAT to next day 06:59:59 EAT.
func (s *DashboardService) GenerateDailySalesReportPDF(ctx context.Context, businessDate string) ([]byte, string, error) {
	loc, err := time.LoadLocation(reportTimezoneName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load timezone: %w", err)
	}

	targetDate, err := resolveBusinessDate(businessDate, loc)
	if err != nil {
		return nil, "", err
	}

	startLocal, endLocal := businessDayWindow(targetDate, loc)

	report, err := s.buildSalesReport(ctx, "Daily Sales Report", targetDate.Format("2006-01-02"), startLocal, endLocal, loc)
	if err != nil {
		return nil, "", err
	}

	pdfBytes, err := renderSalesReportPDF(report, loc)
	if err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("daily-sales-%s.pdf", targetDate.Format("2006-01-02"))
	return pdfBytes, filename, nil
}

// GenerateLast30DaysSalesReportPDF generates a PDF report for the previous 30 completed operational days.
// Window always ends on yesterday business date (not today's in-progress business date).
func (s *DashboardService) GenerateLast30DaysSalesReportPDF(ctx context.Context) ([]byte, string, error) {
	loc, err := time.LoadLocation(reportTimezoneName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load timezone: %w", err)
	}

	nowLocal := time.Now().In(loc)
	currentBusinessDate := currentBusinessDateInLocation(nowLocal, loc)
	endBusinessDate := currentBusinessDate.AddDate(0, 0, -1)

	endStartLocal, endWindowLocal := businessDayWindow(endBusinessDate, loc)
	startLocal := endStartLocal.AddDate(0, 0, -29)

	dateLabel := fmt.Sprintf("%s to %s", startLocal.Format("2006-01-02"), endBusinessDate.Format("2006-01-02"))
	report, err := s.buildSalesReport(ctx, "Last 30 Days Sales Report", dateLabel, startLocal, endWindowLocal, loc)
	if err != nil {
		return nil, "", err
	}

	pdfBytes, err := renderSalesReportPDF(report, loc)
	if err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("sales-30-days-%s.pdf", endBusinessDate.Format("2006-01-02"))
	return pdfBytes, filename, nil
}

func (s *DashboardService) buildSalesReport(
	ctx context.Context,
	title string,
	dateLabel string,
	startLocal time.Time,
	endLocal time.Time,
	loc *time.Location,
) (*core.SalesReport, error) {
	orders, err := s.orderRepo.GetByDateRangeAndStatuses(ctx, startLocal.UTC(), endLocal.UTC(), settledSalesStatuses)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch report orders: %w", err)
	}

	totalRevenue := 0.0
	for _, order := range orders {
		totalRevenue += order.TotalAmount
	}

	avgOrderValue := 0.0
	orderCount := len(orders)
	if orderCount > 0 {
		avgOrderValue = totalRevenue / float64(orderCount)
	}

	statusFilter := make([]string, 0, len(settledSalesStatuses))
	for _, status := range settledSalesStatuses {
		statusFilter = append(statusFilter, string(status))
	}

	domainOrders := make([]core.Order, len(orders))
	for i, order := range orders {
		domainOrders[i] = *order
	}

	report := &core.SalesReport{
		Title:               title,
		DateLabel:           dateLabel,
		Timezone:            reportTimezoneName,
		BusinessDayStart:    "07:00",
		StartAt:             startLocal,
		EndAt:               endLocal,
		GeneratedAt:         time.Now().In(loc),
		TotalRevenue:        totalRevenue,
		OrderCount:          orderCount,
		AverageOrderValue:   avgOrderValue,
		SettledStatusFilter: statusFilter,
		Orders:              domainOrders,
	}

	return report, nil
}

func resolveBusinessDate(dateString string, loc *time.Location) (time.Time, error) {
	if strings.TrimSpace(dateString) == "" {
		nowLocal := time.Now().In(loc)
		return currentBusinessDateInLocation(nowLocal, loc), nil
	}

	parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(dateString), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format, expected YYYY-MM-DD")
	}

	return parsed, nil
}

func currentBusinessDateInLocation(nowLocal time.Time, loc *time.Location) time.Time {
	reference := nowLocal
	if reference.Hour() < businessDayStartHourEAT {
		reference = reference.AddDate(0, 0, -1)
	}

	return time.Date(reference.Year(), reference.Month(), reference.Day(), 0, 0, 0, 0, loc)
}

func businessDayWindow(businessDate time.Time, loc *time.Location) (time.Time, time.Time) {
	start := time.Date(
		businessDate.Year(),
		businessDate.Month(),
		businessDate.Day(),
		businessDayStartHourEAT,
		0,
		0,
		0,
		loc,
	)
	return start, start.Add(24 * time.Hour)
}

func renderSalesReportPDF(report *core.SalesReport, loc *time.Location) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 8, "Destination Cocktails", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 7, report.Title, "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Business Date: %s", report.DateLabel), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Operational Day Start: %s (%s)", report.BusinessDayStart, report.Timezone), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Range: %s to %s", formatReportDateTime(report.StartAt, loc), formatReportDateTime(report.EndAt, loc)), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Statuses Included: %s", strings.Join(report.SettledStatusFilter, ", ")), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Generated At: %s", formatReportDateTime(report.GeneratedAt, loc)), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 7, "Summary", "1", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(95, 7, fmt.Sprintf("Total Sales: %s", formatKsh(report.TotalRevenue)), "1", 0, "L", false, 0, "")
	pdf.CellFormat(95, 7, fmt.Sprintf("Orders: %d", report.OrderCount), "1", 1, "L", false, 0, "")
	pdf.CellFormat(190, 7, fmt.Sprintf("Average Order Value: %s", formatKsh(report.AverageOrderValue)), "1", 1, "L", false, 0, "")
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 7, "Order-Level Detail", "", 1, "L", false, 0, "")

	if len(report.Orders) == 0 {
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 6, "No settled orders found for this report range.", "", 1, "L", false, 0, "")
	} else {
		for i, order := range report.Orders {
			ensurePageSpace(pdf, 35)

			pdf.SetFont("Arial", "B", 10)
			headerLine := fmt.Sprintf(
				"%d) Pickup #%s | %s | %s",
				i+1,
				safeReportValue(order.PickupCode),
				string(order.Status),
				formatReportDateTime(order.CreatedAt, loc),
			)
			pdf.MultiCell(0, 6, headerLine, "", "L", false)

			pdf.SetFont("Arial", "", 10)
			pdf.MultiCell(0, 5, fmt.Sprintf("Phone: %s", safeReportValue(order.CustomerPhone)), "", "L", false)
			pdf.MultiCell(0, 5, fmt.Sprintf("Total: %s | Payment: %s | Reference: %s", formatKsh(order.TotalAmount), safeReportValue(order.PaymentMethod), safeReportValue(order.PaymentRef)), "", "L", false)

			if len(order.Items) == 0 {
				pdf.MultiCell(0, 5, "- No items found", "", "L", false)
			} else {
				for _, item := range order.Items {
					lineTotal := item.PriceAtTime * float64(item.Quantity)
					itemLine := fmt.Sprintf(
						"- %dx %s @ %s = %s",
						item.Quantity,
						safeReportValue(item.ProductName),
						formatKsh(item.PriceAtTime),
						formatKsh(lineTotal),
					)
					pdf.MultiCell(0, 5, itemLine, "", "L", false)
				}
			}

			pdf.CellFormat(0, 1, "", "B", 1, "L", false, 0, "")
			pdf.Ln(1)
		}
	}

	var buffer bytes.Buffer
	if err := pdf.Output(&buffer); err != nil {
		return nil, fmt.Errorf("failed to render PDF: %w", err)
	}

	return buffer.Bytes(), nil
}

func ensurePageSpace(pdf *gofpdf.Fpdf, minSpace float64) {
	pageWidth, pageHeight := pdf.GetPageSize()
	leftMargin, _, rightMargin, bottomMargin := pdf.GetMargins()
	_ = pageWidth
	usableBottom := pageHeight - bottomMargin
	if pdf.GetY()+minSpace > usableBottom {
		pdf.AddPage()
		pdf.SetX(leftMargin)
		pdf.SetRightMargin(rightMargin)
	}
}

func safeReportValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func formatReportDateTime(value time.Time, loc *time.Location) string {
	return value.In(loc).Format("02 Jan 2006 15:04")
}

func formatKsh(amount float64) string {
	return fmt.Sprintf("Ksh %.2f", amount)
}
