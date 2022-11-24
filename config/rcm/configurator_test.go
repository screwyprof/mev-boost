package rcm_test

import (
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/flashbots/mev-boost/config/rcm"
	"github.com/flashbots/mev-boost/config/rcp"
	"github.com/flashbots/mev-boost/config/relay"
	"github.com/flashbots/mev-boost/server"
	"github.com/flashbots/mev-boost/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ server.RelayConfigManager = (*rcm.Configurator)(nil)
	_ rcm.RelayRegistry         = (*relay.Registry)(nil)
)

func TestDefaultConfigManager(t *testing.T) {
	t.Parallel()

	t.Run("it returns all relays for a known validator", func(t *testing.T) {
		t.Parallel()

		// arrange
		validatorPublicKey := testutil.RandomBLSPublicKey(t)
		want := testutil.RandomRelaySet(t, 3)
		configProvider := createMockRelayConfigProvider(withProposerRelays(validatorPublicKey.String(), want.ToStringSlice()))

		sut, err := rcm.NewDefault(rcm.NewRegistryCreator(configProvider))
		require.NoError(t, err)

		// act
		got := sut.RelaysForValidator(validatorPublicKey.String())

		// assert
		assert.ElementsMatch(t, want.ToList(), got)
	})

	t.Run("it returns default relays for an unknown validator", func(t *testing.T) {
		t.Parallel()

		// arrange
		validatorPublicKey := testutil.RandomBLSPublicKey(t)
		want := testutil.RandomRelaySet(t, 3)
		configProvider := createMockRelayConfigProvider(withDefaultRelays(want.ToStringSlice()))

		sut, err := rcm.NewDefault(rcm.NewRegistryCreator(configProvider))
		require.NoError(t, err)

		// act
		got := sut.RelaysForValidator(validatorPublicKey.String())

		// assert
		assert.ElementsMatch(t, want.ToList(), got)
	})

	t.Run("it returns an error if it cannot create the registry", func(t *testing.T) {
		t.Parallel()

		// arrange
		configProvider := createMockRelayConfigProvider(withErr())

		// act
		_, err := rcm.NewDefault(rcm.NewRegistryCreator(configProvider))

		// assert
		assert.ErrorIs(t, err, rcm.ErrCannotFetchRelayConfig)
	})

	t.Run("it returns proposer and default relays", func(t *testing.T) {
		t.Parallel()

		// arrange
		validatorPublicKey := testutil.RandomBLSPublicKey(t)
		proposerRelays := testutil.RandomRelaySet(t, 3)
		defaultRelays := testutil.RandomRelaySet(t, 2)

		want := testutil.JoinSets(proposerRelays, defaultRelays).ToList()
		configProvider := createMockRelayConfigProvider(
			withProposerRelays(validatorPublicKey.String(), proposerRelays.ToStringSlice()),
			withDefaultRelays(defaultRelays.ToStringSlice()))

		sut, err := rcm.NewDefault(rcm.NewRegistryCreator(configProvider))
		require.NoError(t, err)

		// act
		got := sut.AllRelays()

		// assert
		assert.ElementsMatch(t, want, got)
	})

	t.Run("it returns only unique relays", func(t *testing.T) {
		t.Parallel()

		// arrange
		validatorPublicKey := testutil.RandomBLSPublicKey(t)
		proposerRelays := testutil.RelaySetWithRelayHavingTheSameURL(t, 3)
		defaultRelays := testutil.RelaySetWithRelayHavingTheSameURL(t, 2)

		want := testutil.JoinSets(proposerRelays, defaultRelays).ToList()
		configProvider := createMockRelayConfigProvider(
			withProposerRelays(validatorPublicKey.String(), proposerRelays.ToStringSlice()),
			withDefaultRelays(defaultRelays.ToStringSlice()))

		sut, err := rcm.NewDefault(rcm.NewRegistryCreator(configProvider))
		require.NoError(t, err)

		// act
		got := sut.AllRelays()

		// assert
		assert.ElementsMatch(t, want, got)
		assert.Len(t, got, 2)
	})

	t.Run("it uses the previously stored relays if synchronisation error occurs", func(t *testing.T) {
		t.Parallel()

		// arrange
		validatorPublicKey := testutil.RandomBLSPublicKey(t)
		proposerRelays := testutil.RandomRelaySet(t, 3)
		defaultRelays := testutil.RandomRelaySet(t, 2)

		configProvider := onceOnlySuccessfulProvider(validatorPublicKey, proposerRelays, defaultRelays)

		sut, err := rcm.NewDefault(rcm.NewRegistryCreator(configProvider))
		require.NoError(t, err)

		// act
		err = sut.SyncConfig()

		// assert
		require.Error(t, err)
		assertRelaysHaveNotChanged(t, sut)(validatorPublicKey, proposerRelays)
		assertRelaysHaveNotChanged(t, sut)(testutil.RandomBLSPublicKey(t), defaultRelays)
	})

	t.Run("it panics if relay provider is not supplied", func(t *testing.T) {
		t.Parallel()

		assert.Panics(t, func() {
			_, _ = rcm.NewDefault(nil)
		})
	})

	t.Run("it is thread-safe", func(t *testing.T) {
		t.Parallel()

		relays := testutil.RandomRelaySet(t, 5)

		sut, err := rcm.NewDefault(rcm.NewRegistryCreator(rcp.NewDefault(relays).FetchConfig))
		require.NoError(t, err)

		const iterations = 10000
		numOfWorkers := int64(runtime.GOMAXPROCS(0))

		count := runConcurrentlyAndCountFnCalls(numOfWorkers, iterations, func(r *rand.Rand, num int64) {
			randomlyCallRCMMethods(t, sut)(r, num)
		})

		assert.Equal(t, uint64(iterations*numOfWorkers), count)
	})
}

func assertRelaysHaveNotChanged(t *testing.T, sut *rcm.Configurator) func(types.PublicKey, relay.Set) {
	t.Helper()

	return func(pk types.PublicKey, defaultRelays relay.Set) {
		got := sut.RelaysForValidator(pk.String())
		assert.ElementsMatch(t, defaultRelays.ToList(), got)
	}
}

func runConcurrentlyAndCountFnCalls(numOfWorkers, num int64, fn func(*rand.Rand, int64)) uint64 {
	var (
		count uint64
		wg    sync.WaitGroup
	)

	for g := numOfWorkers; g > 0; g-- {
		r := rand.New(rand.NewSource(g))
		wg.Add(1)

		go func(r *rand.Rand) {
			defer wg.Done()

			for n := int64(1); n <= num; n++ {
				fn(r, n)
				atomic.AddUint64(&count, 1)
			}
		}(r)
	}

	wg.Wait()

	return count
}

func randomlyCallRCMMethods(t *testing.T, sut *rcm.Configurator) func(*rand.Rand, int64) {
	t.Helper()

	return func(r *rand.Rand, num int64) {
		switch {
		case r.Int63n(num)%2 == 0:
			require.NoError(t, sut.SyncConfig())
		case r.Int63n(num)%3 == 0:
			sut.RelaysForValidator(testutil.RandomBLSPublicKey(t).String())
		default:
			sut.AllRelays()
		}
	}
}
