package app

import (
	"context"
	"log"
	"sync"

	"bot/internal/app/commands"
	"bot/internal/batchstate"
	"bot/internal/bridgeclient"
	"bot/internal/config"
	"bot/internal/erp"
	"bot/internal/telegram"
	corepkg "core"
)

type App struct {
	cfg                      config.Config
	tg                       *telegram.Client
	erp                      *erp.Client
	qtyReader                *bridgeclient.Client
	batchState               *batchstate.Store
	epcHistory               *EPCHistory
	epcGenerator             *corepkg.EPCGenerator
	log                      *log.Logger
	logRun                   *log.Logger
	logBatch                 *log.Logger
	logCallback              *log.Logger
	logCleanup               *log.Logger
	startInfoMsgByChat       map[int64]int64
	batchPromptMsgByChat     map[int64]int64
	warehousePromptMsgByChat map[int64]int64
	selectionByChat          map[int64]SelectedContext
	itemChoiceByChat         map[int64]itemChoice
	batchChangeMsgByChat     map[int64]int64

	batchMu     sync.Mutex
	batchNextID int64
	batchByChat map[int64]batchSession
}

type batchSession struct {
	id     int64
	cancel context.CancelFunc
}

type SelectedContext struct {
	ItemCode  string
	ItemName  string
	Warehouse string
}

type itemChoice struct {
	ItemCode string
	ItemName string
}

func New(cfg config.Config, logger *log.Logger, runLogger *log.Logger, batchLogger *log.Logger, callbackLogger *log.Logger, cleanupLogger *log.Logger) *App {
	if logger == nil {
		logger = log.Default()
	}
	if runLogger == nil {
		runLogger = logger
	}
	if batchLogger == nil {
		batchLogger = logger
	}
	if callbackLogger == nil {
		callbackLogger = logger
	}
	if cleanupLogger == nil {
		cleanupLogger = logger
	}
	return &App{
		cfg:                      cfg,
		tg:                       telegram.New(cfg.TelegramBotToken),
		erp:                      erp.NewWithReadURL(cfg.ERPURL, cfg.ERPAPIKey, cfg.ERPAPISecret, cfg.ERPReadURL),
		qtyReader:                bridgeclient.New(cfg.BridgeStateFile),
		batchState:               batchstate.New(cfg.BridgeStateFile),
		epcHistory:               NewEPCHistory(),
		epcGenerator:             corepkg.NewEPCGenerator(),
		log:                      logger,
		logRun:                   runLogger,
		logBatch:                 batchLogger,
		logCallback:              callbackLogger,
		logCleanup:               cleanupLogger,
		startInfoMsgByChat:       make(map[int64]int64),
		batchPromptMsgByChat:     make(map[int64]int64),
		warehousePromptMsgByChat: make(map[int64]int64),
		selectionByChat:          make(map[int64]SelectedContext),
		itemChoiceByChat:         make(map[int64]itemChoice),
		batchChangeMsgByChat:     make(map[int64]int64),
		batchByChat:              make(map[int64]batchSession),
	}
}

func (a *App) deps() commands.Deps {
	return commands.Deps{TG: a.tg, ERP: a.erp}
}
