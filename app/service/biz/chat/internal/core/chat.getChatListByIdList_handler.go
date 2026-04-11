/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatGetChatListByIdList
// chat.getChatListByIdList self_id:long id_list:Vector<long> = Vector<MutableChat>;
func (c *ChatCore) ChatGetChatListByIdList(in *chat.TLChatGetChatListByIdList) (*chat.Vector_MutableChat, error) {
	rValueList := &chat.Vector_MutableChat{
		Datas: make([]*mtproto.MutableChat, 0, len(in.IdList)),
	}

	for _, id := range in.IdList {
		var mChat *mtproto.MutableChat
		if in.SelfId != 0 {
			// 带上当前用户，便于只拉取该用户可见的参与者信息；与 chat.getChatBySelfId 行为一致
			mChat, _ = c.svcCtx.Dao.GetMutableChat(c.ctx, id, in.SelfId)
		} else {
			mChat, _ = c.svcCtx.Dao.GetMutableChat(c.ctx, id)
		}
		if mChat != nil {
			rValueList.Datas = append(rValueList.Datas, mChat)
		}
	}

	return rValueList, nil
}
