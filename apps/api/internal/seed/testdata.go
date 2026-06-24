package seed

import "time"

// Fixed UUIDs for deterministic seeding.
const (
	uuidProfile = "a0000000-0000-0000-0000-000000000001"
	uuidJob1    = "b0000000-0000-0000-0000-000000000001"
	uuidJob2    = "b0000000-0000-0000-0000-000000000002"
	uuidJob3    = "b0000000-0000-0000-0000-000000000003"
	uuidJob4    = "b0000000-0000-0000-0000-000000000004"
	uuidJob5    = "b0000000-0000-0000-0000-000000000005"
	uuidJob6    = "b0000000-0000-0000-0000-000000000006"
	uuidJob7    = "b0000000-0000-0000-0000-000000000007"
	uuidJob8    = "b0000000-0000-0000-0000-000000000008"
	uuidJob9    = "b0000000-0000-0000-0000-000000000009"
	uuidJob10   = "b0000000-0000-0000-0000-000000000010"
	uuidJob11   = "b0000000-0000-0000-0000-000000000011"
	uuidJob12   = "b0000000-0000-0000-0000-000000000012"
	uuidJob13   = "b0000000-0000-0000-0000-000000000013"
	uuidJob14   = "b0000000-0000-0000-0000-000000000014"
	uuidJob15   = "b0000000-0000-0000-0000-000000000015"
	uuidJob16   = "b0000000-0000-0000-0000-000000000016"
	uuidJob17   = "b0000000-0000-0000-0000-000000000017"
	uuidJob18   = "b0000000-0000-0000-0000-000000000018"
	uuidJob19   = "b0000000-0000-0000-0000-000000000019"
	uuidJob20   = "b0000000-0000-0000-0000-000000000020"
	uuidSearch1 = "c0000000-0000-0000-0000-000000000001"
	uuidSearch2 = "c0000000-0000-0000-0000-000000000002"
	uuidSearch3 = "c0000000-0000-0000-0000-000000000003"
	uuidSearch4 = "c0000000-0000-0000-0000-000000000004"
	uuidApp1    = "d0000000-0000-0000-0000-000000000001"
	uuidApp2    = "d0000000-0000-0000-0000-000000000002"
	uuidApp3    = "d0000000-0000-0000-0000-000000000003"
	uuidApp4    = "d0000000-0000-0000-0000-000000000004"
	uuidApp5    = "d0000000-0000-0000-0000-000000000005"
	uuidApp6    = "d0000000-0000-0000-0000-000000000006"
	uuidApp7    = "d0000000-0000-0000-0000-000000000007"
	uuidApp8    = "d0000000-0000-0000-0000-000000000008"
	uuidApp9    = "d0000000-0000-0000-0000-000000000009"
	uuidApp10   = "d0000000-0000-0000-0000-000000000010"
	uuidDoc1    = "e0000000-0000-0000-0000-000000000001"
	uuidDoc2    = "e0000000-0000-0000-0000-000000000002"
	uuidDoc3    = "e0000000-0000-0000-0000-000000000003"
	uuidDoc4    = "e0000000-0000-0000-0000-000000000004"
	uuidDoc5    = "e0000000-0000-0000-0000-000000000005"
	uuidMatch1  = "f0000000-0000-0000-0000-000000000001"
	uuidMatch2  = "f0000000-0000-0000-0000-000000000002"
	uuidMatch3  = "f0000000-0000-0000-0000-000000000003"
	uuidMatch4  = "f0000000-0000-0000-0000-000000000004"
	uuidMatch5  = "f0000000-0000-0000-0000-000000000005"
	uuidMatch6  = "f0000000-0000-0000-0000-000000000006"
	uuidMatch7  = "f0000000-0000-0000-0000-000000000007"
	uuidMatch8  = "f0000000-0000-0000-0000-000000000008"
	uuidMatch9  = "f0000000-0000-0000-0000-000000000009"
	uuidMatch10 = "f0000000-0000-0000-0000-000000000010"
	uuidRun1    = "aa000000-0000-0000-0000-000000000001"
	uuidRun2    = "aa000000-0000-0000-0000-000000000002"
	uuidRun3    = "aa000000-0000-0000-0000-000000000003"
	uuidRun4    = "aa000000-0000-0000-0000-000000000004"
	uuidRun5    = "aa000000-0000-0000-0000-000000000005"
	uuidRun6    = "aa000000-0000-0000-0000-000000000006"
	uuidRun7    = "aa000000-0000-0000-0000-000000000007"
	uuidRun8    = "aa000000-0000-0000-0000-000000000008"
	uuidRun9    = "aa000000-0000-0000-0000-000000000009"
	uuidRun10   = "aa000000-0000-0000-0000-000000000010"
	uuidRun11   = "aa000000-0000-0000-0000-000000000011"
	uuidRun12   = "aa000000-0000-0000-0000-000000000012"
	uuidRun13   = "aa000000-0000-0000-0000-000000000013"
	uuidRun14   = "aa000000-0000-0000-0000-000000000014"
	uuidRun15   = "aa000000-0000-0000-0000-000000000015"
	uuidSub1    = "ab000000-0000-0000-0000-000000000001"
	uuidSub2    = "ab000000-0000-0000-0000-000000000002"
	uuidSub3    = "ab000000-0000-0000-0000-000000000003"
)

// jobSeed is the fixture data for a single seed job.
type jobSeed struct {
	UUID      string
	SourceKey string
	Title     string
	Company   string
	Location  string
	Remote    bool
	SalaryRaw string
	URL       string
	Desc      string
	Status    string
	PostedAt  time.Time
}

var jobs = []jobSeed{
	{
		UUID: uuidJob1, SourceKey: "djinni", Title: "Senior Go Developer",
		Company: "NovaTech", Location: "Kyiv, Ukraine", Remote: true,
		SalaryRaw: "$4000–$6000/mo", URL: "https://djinni.io/jobs/novatech-go-senior",
		Desc:   "We are looking for a senior Go developer to build and maintain high-performance microservices. You will work on our core platform processing millions of daily requests, designing APIs, and mentoring junior engineers. Strong understanding of concurrency, context propagation, and graceful shutdown is required.",
		Status: "shortlisted", PostedAt: now().AddDate(0, 0, -3),
	},
	{
		UUID: uuidJob2, SourceKey: "djinni", Title: "React / Node.js Engineer",
		Company: "DataFlow", Location: "Remote", Remote: true,
		SalaryRaw: "$3000–$5000/mo", URL: "https://djinni.io/jobs/dataflow-react-node",
		Desc:   "Join our team to build real-time data visualization dashboards using React, TypeScript, and Node.js. Experience with WebSocket, D3.js or Recharts, and PostgreSQL is a plus. We ship weekly and value clean, tested code.",
		Status: "found", PostedAt: now().AddDate(0, 0, -5),
	},
	{
		UUID: uuidJob3, SourceKey: "dou", Title: "Backend Developer (Go)",
		Company: "CloudScale", Location: "Kyiv, Ukraine", Remote: false,
		SalaryRaw: "120k–180k UAH/mo", URL: "https://dou.ua/jobs/cloudscale-go-backend",
		Desc:   "CloudScale is hiring a Go backend developer to work on our SaaS infrastructure platform. You will design RESTful APIs, write unit and integration tests, and collaborate with DevOps on Kubernetes deployments. Go 1.22+, gRPC, and SQL experience required.",
		Status: "applied", PostedAt: now().AddDate(0, 0, -2),
	},
	{
		UUID: uuidJob4, SourceKey: "dou", Title: "Full-Stack Engineer",
		Company: "PixelWorks", Location: "Lviv, Ukraine", Remote: true,
		SalaryRaw: "90k–140k UAH/mo", URL: "https://dou.ua/jobs/pixelworks-fullstack",
		Desc:   "We need a full-stack engineer comfortable with Go on the backend and React/Next.js on the frontend. You will own features end-to-end, from database schema to pixel-perfect UI. Experience with Tailwind CSS and PostgreSQL preferred.",
		Status: "found", PostedAt: now().AddDate(0, 0, -7),
	},
	{
		UUID: uuidJob5, SourceKey: "adzuna", Title: "Senior Software Engineer",
		Company: "TechVentures", Location: "London, UK", Remote: false,
		SalaryRaw: "£70k–£90k/yr", URL: "https://adzuna.co.uk/jobs/techventures-senior-eng",
		Desc:   "TechVentures is looking for a senior software engineer to join our fintech platform team. You will build scalable payment processing systems, work with event-driven architectures, and contribute to our open-source tooling. Python or Go, Kafka, and AWS experience valued.",
		Status: "applied", PostedAt: now().AddDate(0, 0, -4),
	},
	{
		UUID: uuidJob6, SourceKey: "adzuna", Title: "DevOps Engineer",
		Company: "InfraCore", Location: "Remote", Remote: true,
		SalaryRaw: "£65k–£85k/yr", URL: "https://adzuna.co.uk/jobs/infracore-devops",
		Desc:   "InfraCore needs a DevOps engineer to manage and improve our CI/CD pipelines, Kubernetes clusters, and monitoring stack. Terraform, GitHub Actions, Prometheus/Grafana, and strong Linux skills required. On-call rotation included.",
		Status: "found", PostedAt: now().AddDate(0, 0, -6),
	},
	{
		UUID: uuidJob7, SourceKey: "remotive", Title: "Senior React Developer",
		Company: "RemoteFirst", Location: "Remote (UTC±5)", Remote: true,
		SalaryRaw: "$90k–$120k/yr", URL: "https://remotive.com/remote-jobs/remotefirst-react",
		Desc:   "RemoteFirst is building the next generation of collaborative tools. We need a senior React developer with deep knowledge of hooks, state management (Zustand or Jotai), and performance optimization. TypeScript required; GraphQL is a plus.",
		Status: "shortlisted", PostedAt: now().AddDate(0, 0, -1),
	},
	{
		UUID: uuidJob8, SourceKey: "remotive", Title: "Rust Systems Engineer",
		Company: "OxideCo", Location: "Remote (UTC±3)", Remote: true,
		SalaryRaw: "$100k–$140k/yr", URL: "https://remotive.com/remote-jobs/oxideco-rust",
		Desc:   "OxideCo is building a distributed storage engine in Rust. We need systems-level experience: async runtimes (tokio), memory-safe concurrency, and low-level networking. Prior Rust experience required; C/C++ background is a plus.",
		Status: "found", PostedAt: now().AddDate(0, 0, -8),
	},
	{
		UUID: uuidJob9, SourceKey: "arbeitnow", Title: "Python / Django Developer",
		Company: "WebStack", Location: "Berlin, Germany", Remote: false,
		SalaryRaw: "€55k–€75k/yr", URL: "https://arbeitnow.com/jobs/webstack-python-django",
		Desc:   "WebStack is a growing SaaS company looking for a Python/Django developer. You will build REST APIs, manage PostgreSQL databases, and integrate with third-party services. Celery, Redis, and Docker experience preferred.",
		Status: "applied", PostedAt: now().AddDate(0, 0, -3),
	},
	{
		UUID: uuidJob10, SourceKey: "arbeitnow", Title: "Kubernetes Engineer",
		Company: "CloudNative", Location: "Remote", Remote: true,
		SalaryRaw: "€70k–€90k/yr", URL: "https://arbeitnow.com/jobs/cloudnative-k8s",
		Desc:   "CloudNative is seeking a Kubernetes engineer to design and operate production clusters across multiple cloud providers. Helm, Terraform, service mesh (Istio/Linkerd), and strong networking skills required.",
		Status: "found", PostedAt: now().AddDate(0, 0, -10),
	},
	{
		UUID: uuidJob11, SourceKey: "jooble", Title: "Golang Microservices Developer",
		Company: "ScaleUp", Location: "Warsaw, Poland", Remote: true,
		SalaryRaw: "18k–25k PLN/mo", URL: "https://jooble.org/desc/golang-microservices-scaleup",
		Desc:   "ScaleUp is building a high-throughput event processing platform. We need a Go developer experienced with microservices patterns: circuit breakers, retries, distributed tracing. gRPC, Kafka, and Kubernetes experience required.",
		Status: "shortlisted", PostedAt: now().AddDate(0, 0, -2),
	},
	{
		UUID: uuidJob12, SourceKey: "jooble", Title: "Frontend Tech Lead",
		Company: "UXStudio", Location: "Remote", Remote: true,
		SalaryRaw: "$80k–$110k/yr", URL: "https://jooble.org/desc/frontend-lead-uxstudio",
		Desc:   "UXStudio is looking for a frontend tech lead to guide our React/TypeScript codebase. You will set coding standards, review PRs, and mentor a team of 4 frontend developers. Deep knowledge of accessibility, testing, and performance required.",
		Status: "found", PostedAt: now().AddDate(0, 0, -5),
	},
	{
		UUID: uuidJob13, SourceKey: "jobspy", Title: "Backend Engineer (Python)",
		Company: "FinEdge", Location: "New York, NY", Remote: false,
		SalaryRaw: "$110k–$140k/yr", URL: "https://linkedin.com/jobs/finedge-backend-engineer",
		Desc:   "FinEdge is a fintech startup building real-time trading analytics. We need a backend engineer with Python, FastAPI, and PostgreSQL experience. Familiarity with WebSockets, async programming, and financial data is a plus.",
		Status: "interview", PostedAt: now().AddDate(0, 0, -6),
	},
	{
		UUID: uuidJob14, SourceKey: "jobspy", Title: "Site Reliability Engineer",
		Company: "MegaCloud", Location: "San Francisco, CA", Remote: true,
		SalaryRaw: "$130k–$160k/yr", URL: "https://indeed.com/jobs/megacloud-sre",
		Desc:   "MegaCloud is hiring an SRE to ensure 99.99% uptime for our multi-region Kubernetes deployment. You will build observability tooling, define SLOs, and automate incident response. Go or Python, Prometheus, and strong scripting skills required.",
		Status: "shortlisted", PostedAt: now().AddDate(0, 0, -1),
	},
	{
		UUID: uuidJob15, SourceKey: "jobspy", Title: "Machine Learning Engineer",
		Company: "NeuralPath", Location: "Toronto, Canada", Remote: true,
		SalaryRaw: "CAD $120k–$150k/yr", URL: "https://glassdoor.com/jobs/neuralpath-ml-engineer",
		Desc:   "NeuralPath is building production ML pipelines for natural language processing. We need experience with PyTorch, transformers, MLOps (MLflow/Weights & Biases), and model serving (Triton/torchserve). Strong Python and ML fundamentals required.",
		Status: "offer", PostedAt: now().AddDate(0, 0, -4),
	},
	{
		UUID: uuidJob16, SourceKey: "djinni", Title: "DevOps / Platform Engineer",
		Company: "ByteForge", Location: "Remote", Remote: true,
		SalaryRaw: "$3500–$5500/mo", URL: "https://djinni.io/jobs/byteforge-devops",
		Desc:   "ByteForge is looking for a platform engineer to build internal developer tooling. You will manage AWS infrastructure with Terraform, set up CI/CD pipelines, and create self-service deployment tooling for engineering teams.",
		Status: "applied", PostedAt: now().AddDate(0, 0, -5),
	},
	{
		UUID: uuidJob17, SourceKey: "dou", Title: "Junior Go Developer",
		Company: "StartupLab", Location: "Kharkiv, Ukraine", Remote: true,
		SalaryRaw: "50k–80k UAH/mo", URL: "https://dou.ua/jobs/startuplab-junior-go",
		Desc:   "StartupLab is hiring a junior Go developer to join our small but mighty team. You will learn by building real features: REST APIs, database migrations, and automated tests. Mentoring provided. Go basics, Git, and SQL knowledge expected.",
		Status: "found", PostedAt: now().AddDate(0, 0, -9),
	},
	{
		UUID: uuidJob18, SourceKey: "adzuna", Title: "Technical Writer",
		Company: "DocuFlow", Location: "Remote (EU)", Remote: true,
		SalaryRaw: "€45k–€60k/yr", URL: "https://adzuna.co.uk/jobs/docuflow-tech-writer",
		Desc:   "DocuFlow needs a technical writer who can explain complex distributed systems concepts clearly. You will write API documentation, architecture guides, and onboarding materials. Markdown, OpenAPI/Swagger, and basic programming knowledge required.",
		Status: "applied", PostedAt: now().AddDate(0, 0, -7),
	},
	{
		UUID: uuidJob19, SourceKey: "remotive", Title: "iOS Developer",
		Company: "AppCraft", Location: "Remote (UTC±7)", Remote: true,
		SalaryRaw: "$85k–$115k/yr", URL: "https://remotive.com/remote-jobs/appcraft-ios",
		Desc:   "AppCraft is building a consumer social app. We need an iOS developer fluent in Swift, SwiftUI, and Combine. Experience with Core Data, push notifications, and App Store deployment required.",
		Status: "interview", PostedAt: now().AddDate(0, 0, -3),
	},
	{
		UUID: uuidJob20, SourceKey: "jooble", Title: "Data Engineer",
		Company: "InsightLabs", Location: "Amsterdam, Netherlands", Remote: false,
		SalaryRaw: "€60k–€80k/yr", URL: "https://jooble.org/desc/data-engineer-insightlabs",
		Desc:   "InsightLabs is hiring a data engineer to build and maintain our analytics platform. You will design ETL pipelines with Airflow, manage data warehouses in Snowflake, and build dbt models. Python, SQL, and cloud data services (AWS/GCP) required.",
		Status: "found", PostedAt: now().AddDate(0, 0, -11),
	},
}

// daysAgo returns a time.Time that is n days before now.
func now() time.Time { return time.Now().UTC() }

func daysAgo(n int) time.Time { return now().AddDate(0, 0, -n) }

func daysFromNow(n int) time.Time { return now().AddDate(0, 0, n) }

func strPtr(s string) *string { return &s }

func int32Ptr(i int32) *int32 { return &i }

func boolPtr(b bool) *bool { return &b }
