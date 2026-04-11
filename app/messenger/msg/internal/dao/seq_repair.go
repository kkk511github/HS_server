package dao

import (
	"context"
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

const seqRepairMaxAttempts = 3

func (d *Dao) NextMessageBoxId(ctx context.Context, userId int64) int32 {
	for attempt := 0; attempt < seqRepairMaxAttempts; attempt++ {
		nextID := d.IDGenClient2.NextMessageBoxId(ctx, userId)
		if nextID == 0 {
			return 0
		}

		maxID, ok := d.loadMessageBoxSeqFloor(userId)
		if !ok {
			var err error
			maxID, err = d.getLatestMessageBoxId(ctx, userId)
			if err != nil {
				logx.WithContext(ctx).Errorf("NextMessageBoxId - query latest message box id failed, user_id: %d, err: %v", userId, err)
				return nextID
			}
			d.storeMessageBoxSeqFloor(userId, maxID)
		}
		if nextID > maxID {
			d.storeMessageBoxSeqFloor(userId, nextID)
			return nextID
		}

		logx.WithContext(ctx).Errorf("NextMessageBoxId - repaired stale redis sequence, user_id: %d, allocated: %d, max_db: %d", userId, nextID, maxID)
		d.IDGenClient2.SetCurrentMessageBoxId(ctx, userId, maxID)
	}

	return d.IDGenClient2.NextMessageBoxId(ctx, userId)
}

func (d *Dao) NextPtsId(ctx context.Context, userId int64) int32 {
	for attempt := 0; attempt < seqRepairMaxAttempts; attempt++ {
		nextPts := d.IDGenClient2.NextPtsId(ctx, userId)
		if nextPts == 0 {
			return 0
		}

		maxPts, ok := d.loadPtsSeqFloor(userId)
		if !ok {
			var err error
			maxPts, err = d.getLatestPtsId(ctx, userId)
			if err != nil {
				logx.WithContext(ctx).Errorf("NextPtsId - query latest pts failed, user_id: %d, err: %v", userId, err)
				return nextPts
			}
			d.storePtsSeqFloor(userId, maxPts)
		}
		if nextPts > maxPts {
			d.storePtsSeqFloor(userId, nextPts)
			return nextPts
		}

		logx.WithContext(ctx).Errorf("NextPtsId - repaired stale redis sequence, user_id: %d, allocated: %d, max_db: %d", userId, nextPts, maxPts)
		d.IDGenClient2.SetCurrentPtsId(ctx, userId, maxPts)
	}

	return d.IDGenClient2.NextPtsId(ctx, userId)
}

func (d *Dao) getLatestMessageBoxId(ctx context.Context, userId int64) (int32, error) {
	var row dataobject.MessagesDO

	query := "select user_message_box_id from " + d.MessagesDAO.CalcTableName(userId) +
		" where user_id = ? order by user_message_box_id desc limit 1"
	err := d.DB.QueryRowPartial(ctx, &row, query, userId)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}

	return row.UserMessageBoxId, nil
}

func (d *Dao) getLatestPtsId(ctx context.Context, userId int64) (int32, error) {
	row, err := d.UserPtsUpdatesDAO.SelectLastPts(ctx, userId)
	if err != nil || row == nil {
		return 0, err
	}

	return row.Pts, nil
}

func (d *Dao) loadMessageBoxSeqFloor(userId int64) (int32, bool) {
	v, ok := d.messageBoxSeqFloor.Load(userId)
	if !ok {
		return 0, false
	}
	return v.(int32), true
}

func (d *Dao) storeMessageBoxSeqFloor(userId int64, v int32) {
	d.messageBoxSeqFloor.Store(userId, v)
}

func (d *Dao) loadPtsSeqFloor(userId int64) (int32, bool) {
	v, ok := d.ptsSeqFloor.Load(userId)
	if !ok {
		return 0, false
	}
	return v.(int32), true
}

func (d *Dao) storePtsSeqFloor(userId int64, v int32) {
	d.ptsSeqFloor.Store(userId, v)
}
