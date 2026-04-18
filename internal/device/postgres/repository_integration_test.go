//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Pedrohsbessa/devices-api/internal/device"
	"github.com/Pedrohsbessa/devices-api/internal/device/postgres"
)

const migrationPath = "../../../migrations/000001_create_devices_table.up.sql"

// pool is shared by every test in this package. TestMain owns its lifecycle.
var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("devices_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.WithInitScripts(migrationPath),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		os.Exit(1)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = ctr.Terminate(context.Background())
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}

	pool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		_ = ctr.Terminate(context.Background())
		fmt.Fprintf(os.Stderr, "pgx pool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	pool.Close()
	_ = ctr.Terminate(context.Background())
	os.Exit(code)
}

type deviceSpec struct {
	name, brand string
	state       device.State
}

// newRepo returns a Repository against a freshly truncated devices table.
// Tests using this helper must not call t.Parallel().
func newRepo(t *testing.T) *postgres.Repository {
	t.Helper()
	_, err := pool.Exec(context.Background(), "TRUNCATE TABLE devices")
	require.NoError(t, err)
	return postgres.NewRepository(pool)
}

func mustNewDevice(t *testing.T, name, brand string) *device.Device {
	t.Helper()
	d, err := device.NewDevice(name, brand, time.Now().UTC())
	require.NoError(t, err)
	return d
}

func seed(t *testing.T, repo *postgres.Repository, specs ...deviceSpec) []*device.Device {
	t.Helper()
	ctx := context.Background()
	out := make([]*device.Device, 0, len(specs))
	for _, s := range specs {
		d := mustNewDevice(t, s.name, s.brand)
		if s.state != device.StateAvailable {
			require.NoError(t, d.ChangeState(s.state))
		}
		require.NoError(t, repo.Create(ctx, d))
		out = append(out, d)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// Create / GetByID
// ---------------------------------------------------------------------------

func TestRepository_Create_GetByID_Roundtrip(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	d := mustNewDevice(t, "laptop", "lenovo")
	require.NoError(t, repo.Create(ctx, d))

	got, err := repo.GetByID(ctx, d.ID())
	require.NoError(t, err)
	require.Equal(t, d.ID(), got.ID())
	require.Equal(t, d.Name(), got.Name())
	require.Equal(t, d.Brand(), got.Brand())
	require.Equal(t, d.State(), got.State())
	require.True(t, got.CreatedAt().Equal(d.CreatedAt()))
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.GetByID(context.Background(), uuid.New())
	require.ErrorIs(t, err, device.ErrDeviceNotFound)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestRepository_Update_Roundtrip(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	d := mustNewDevice(t, "laptop", "lenovo")
	require.NoError(t, repo.Create(ctx, d))

	require.NoError(t, d.Rename("pro"))
	require.NoError(t, d.ChangeBrand("dell"))
	require.NoError(t, d.ChangeState(device.StateInUse))
	require.NoError(t, repo.Update(ctx, d))

	got, err := repo.GetByID(ctx, d.ID())
	require.NoError(t, err)
	require.Equal(t, "pro", got.Name())
	require.Equal(t, "dell", got.Brand())
	require.Equal(t, device.StateInUse, got.State())
	require.True(t, got.CreatedAt().Equal(d.CreatedAt()), "createdAt must not change on update")
}

func TestRepository_Update_NotFound(t *testing.T) {
	repo := newRepo(t)
	d := mustNewDevice(t, "laptop", "lenovo")

	err := repo.Update(context.Background(), d)
	require.ErrorIs(t, err, device.ErrDeviceNotFound)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestRepository_Delete_Roundtrip(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	d := mustNewDevice(t, "laptop", "lenovo")
	require.NoError(t, repo.Create(ctx, d))
	require.NoError(t, repo.Delete(ctx, d.ID()))

	_, err := repo.GetByID(ctx, d.ID())
	require.ErrorIs(t, err, device.ErrDeviceNotFound)
}

func TestRepository_Delete_NotFound(t *testing.T) {
	repo := newRepo(t)
	err := repo.Delete(context.Background(), uuid.New())
	require.ErrorIs(t, err, device.ErrDeviceNotFound)
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestRepository_List_EmptyReturnsEmpty(t *testing.T) {
	repo := newRepo(t)
	got, err := repo.List(context.Background(), device.ListFilter{})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRepository_List_FilterByBrand(t *testing.T) {
	repo := newRepo(t)
	seed(t, repo,
		deviceSpec{"a", "lenovo", device.StateAvailable},
		deviceSpec{"b", "lenovo", device.StateInUse},
		deviceSpec{"c", "dell", device.StateAvailable},
	)

	got, err := repo.List(context.Background(), device.ListFilter{Brand: ptr("lenovo")})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, d := range got {
		require.Equal(t, "lenovo", d.Brand())
	}
}

func TestRepository_List_FilterByState(t *testing.T) {
	repo := newRepo(t)
	seed(t, repo,
		deviceSpec{"a", "lenovo", device.StateAvailable},
		deviceSpec{"b", "lenovo", device.StateInUse},
		deviceSpec{"c", "dell", device.StateInUse},
	)

	got, err := repo.List(context.Background(), device.ListFilter{State: ptr(device.StateInUse)})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, d := range got {
		require.Equal(t, device.StateInUse, d.State())
	}
}

func TestRepository_List_FilterByBrandAndState(t *testing.T) {
	repo := newRepo(t)
	seed(t, repo,
		deviceSpec{"a", "lenovo", device.StateAvailable},
		deviceSpec{"b", "lenovo", device.StateInUse},
		deviceSpec{"c", "lenovo", device.StateInactive},
		deviceSpec{"d", "dell", device.StateAvailable},
		deviceSpec{"e", "dell", device.StateInUse},
	)

	got, err := repo.List(context.Background(), device.ListFilter{
		Brand: ptr("lenovo"),
		State: ptr(device.StateInUse),
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "b", got[0].Name())
}

func TestRepository_List_OrderedMostRecentFirst(t *testing.T) {
	repo := newRepo(t)
	seeded := seed(t, repo,
		deviceSpec{"first", "lenovo", device.StateAvailable},
		deviceSpec{"second", "lenovo", device.StateAvailable},
		deviceSpec{"third", "lenovo", device.StateAvailable},
	)

	got, err := repo.List(context.Background(), device.ListFilter{})
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, seeded[2].ID(), got[0].ID())
	require.Equal(t, seeded[1].ID(), got[1].ID())
	require.Equal(t, seeded[0].ID(), got[2].ID())
}

func TestRepository_List_Pagination(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	seeded := make([]*device.Device, 5)
	for i := range seeded {
		d := mustNewDevice(t, fmt.Sprintf("d%d", i), "lenovo")
		require.NoError(t, repo.Create(ctx, d))
		seeded[i] = d
	}

	first, err := repo.List(ctx, device.ListFilter{Limit: 2})
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.Equal(t, seeded[4].ID(), first[0].ID())
	require.Equal(t, seeded[3].ID(), first[1].ID())

	second, err := repo.List(ctx, device.ListFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	require.Len(t, second, 2)
	require.Equal(t, seeded[2].ID(), second[0].ID())
	require.Equal(t, seeded[1].ID(), second[1].ID())
}

func TestRepository_List_LimitClamp(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	// Bulk insert via SQL to keep the test fast; v4 uuids are fine here
	// because this test only counts rows, not ordering.
	_, err := pool.Exec(ctx, `
		INSERT INTO devices (id, name, brand, state, created_at)
		SELECT gen_random_uuid(), 'd' || n::text, 'lenovo', 'available', now()
		FROM generate_series(1, 201) n
	`)
	require.NoError(t, err)

	got, err := repo.List(ctx, device.ListFilter{Limit: 9999})
	require.NoError(t, err)
	require.Len(t, got, 200, "limit must be clamped to the repository maximum")
}
