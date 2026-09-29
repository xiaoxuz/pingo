package conn

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/pingo/hub/internal/model"
	"github.com/pingo/hub/internal/service"
	"github.com/pingo/hub/pkg/protocol"
)

// Router 处理上行 WebSocket 消息的路由分发
type Router struct {
	manager             *Manager
	agentService        *service.AgentService
	friendshipService   *service.FriendshipService
	conversationService *service.ConversationService
	messageService      *service.MessageService
}

func NewRouter(
	manager *Manager,
	agentService *service.AgentService,
	friendshipService *service.FriendshipService,
	conversationService *service.ConversationService,
	messageService *service.MessageService,
) *Router {
	return &Router{
		manager:             manager,
		agentService:        agentService,
		friendshipService:   friendshipService,
		conversationService: conversationService,
		messageService:      messageService,
	}
}

// HandleMessage 处理一条上行消息
func (r *Router) HandleMessage(conn *Connection, env protocol.Envelope) {
	ctx := context.Background()
	agentID := conn.AgentID()

	switch env.Type {
	case protocol.TypeHeartbeatPing:
		conn.Send(protocol.NewResponse(protocol.TypeHeartbeatPong, env.RequestID, agentID, map[string]interface{}{}))
		r.agentService.Heartbeat(ctx, agentID)
		r.manager.RefreshHeartbeat(agentID)

	case protocol.TypeAgentUpdateStatus:
		r.handleAgentUpdateStatus(ctx, conn, env)
	case protocol.TypeAgentUpdateCapabilities:
		r.handleAgentUpdateCapabilities(ctx, conn, env)
	case protocol.TypeAgentUpdateOwner:
		r.handleAgentUpdateOwner(ctx, conn, env)
	case protocol.TypeAgentUpdateProfile:
		r.handleAgentUpdateProfile(ctx, conn, env)
	case protocol.TypeAgentUpdateAvailability:
		r.handleAgentUpdateAvailability(ctx, conn, env)

	case protocol.TypeFriendRequest:
		r.handleFriendRequest(ctx, conn, env)
	case protocol.TypeFriendAccept:
		r.handleFriendAccept(ctx, conn, env)
	case protocol.TypeFriendReject:
		r.handleFriendReject(ctx, conn, env)
	case protocol.TypeFriendRemove:
		r.handleFriendRemove(ctx, conn, env)
	case protocol.TypeFriendBlock:
		r.handleFriendBlock(ctx, conn, env)
	case protocol.TypeFriendSetNickname:
		r.handleFriendSetNickname(ctx, conn, env)
	case protocol.TypeFriendSetGroup:
		r.handleFriendSetGroup(ctx, conn, env)
	case protocol.TypeFriendSetTrust:
		r.handleFriendSetTrust(ctx, conn, env)

	case protocol.TypeConvStartDirect:
		r.handleConvStartDirect(ctx, conn, env)
	case protocol.TypeConvCreateGroup:
		r.handleConvCreateGroup(ctx, conn, env)
	case protocol.TypeConvInvite:
		r.handleConvInvite(ctx, conn, env)
	case protocol.TypeConvLeave:
		r.handleConvLeave(ctx, conn, env)
	case protocol.TypeConvClose:
		r.handleConvClose(ctx, conn, env)

	case protocol.TypeMsgSend:
		r.handleMsgSend(ctx, conn, env)

	case protocol.TypeDiscoverSearch:
		r.handleDiscoverSearch(ctx, conn, env)

	case protocol.TypeSyncConversations:
		r.handleSyncConversations(ctx, conn, env)
	case protocol.TypeSyncMessages:
		r.handleSyncMessages(ctx, conn, env)
	case protocol.TypeSyncFriends:
		r.handleSyncFriends(ctx, conn, env)
	case protocol.TypeSyncFriendRequests:
		r.handleSyncFriendRequests(ctx, conn, env)

	default:
		log.Printf("WARN: unknown message type: %s from agent %s", env.Type, agentID)
		conn.Send(protocol.NewError("error", env.RequestID, agentID, protocol.ErrBadRequest, "unknown message type: "+env.Type))
	}
}

func (r *Router) handleAgentUpdateStatus(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.AgentUpdateStatusRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	if err := r.agentService.UpdateStatus(ctx, conn.AgentID(), req.StatusText); err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("agent.update_status.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleAgentUpdateCapabilities(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.AgentUpdateCapabilitiesRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	caps := make([]model.Capability, 0, len(req.Capabilities))
	for _, c := range req.Capabilities {
		caps = append(caps, model.Capability{Skill: c.Skill, Tags: c.Tags})
	}
	if err := r.agentService.UpdateCapabilities(ctx, conn.AgentID(), caps); err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("agent.update_capabilities.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleAgentUpdateOwner(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.AgentUpdateOwnerRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	if err := r.agentService.UpdateOwner(ctx, conn.AgentID(), req.OwnerName, req.OwnerEmail); err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("agent.update_owner.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleAgentUpdateProfile(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.AgentUpdateProfileRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	if err := r.agentService.UpdateProfile(ctx, conn.AgentID(), req.Name, req.AvatarURL, req.StatusText); err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("agent.update_profile.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleAgentUpdateAvailability(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.AgentUpdateAvailabilityRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	availability := model.Availability{
		Mode:                       req.Availability.Mode,
		OnlineHours:                req.Availability.OnlineHours,
		Timezone:                   req.Availability.Timezone,
		MaxConcurrentConversations: req.Availability.MaxConcurrentConversations,
	}
	if err := r.agentService.UpdateAvailability(ctx, conn.AgentID(), availability); err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("agent.update_availability.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleFriendRequest(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendRequestRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	friendship, err := r.friendshipService.SendRequest(ctx, conn.AgentID(), req.Target, req.Message)
	if err != nil {
		code := protocol.ErrInternal
		switch err {
		case service.ErrAlreadyFriends:
			code = protocol.ErrAlreadyFriends
		case service.ErrAgentNotFound:
			code = protocol.ErrAgentNotFound
		case service.ErrBlocked:
			code = protocol.ErrBlocked
		case service.ErrPendingRequest:
			code = protocol.ErrConflict
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	targetConn, online := r.manager.GetConnection(req.Target)
	if online {
		fromAgent, _ := r.agentService.GetByID(ctx, conn.AgentID())
		name := conn.AgentID()
		if fromAgent != nil {
			name = fromAgent.Name
		}
		notify := protocol.FriendRequestNotify{
			FromAgent: conn.AgentID(),
			Name:      name,
			Message:   req.Message,
			CreatedAt: friendship.CreatedAt.Format(time.RFC3339),
		}
		targetConn.Send(protocol.NewNotify(protocol.TypeFriendRequestNotify, req.Target, notify))
	}

	conn.Send(protocol.NewResponse("friend.request.resp", env.RequestID, conn.AgentID(), map[string]interface{}{
		"target": req.Target,
		"status": "pending",
	}))
}

func (r *Router) handleFriendAccept(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendAcceptRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.friendshipService.Accept(ctx, conn.AgentID(), req.Target)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}

	targetConn, online := r.manager.GetConnection(req.Target)
	if online {
		agent, _ := r.agentService.GetByID(ctx, conn.AgentID())
		name := conn.AgentID()
		if agent != nil {
			name = agent.Name
		}
		notify := protocol.FriendAcceptedNotify{
			FromAgent: conn.AgentID(),
			Name:      name,
		}
		targetConn.Send(protocol.NewNotify(protocol.TypeFriendAcceptedNotify, req.Target, notify))
	}

	conn.Send(protocol.NewResponse("friend.accept.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleFriendReject(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendRejectRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.friendshipService.Reject(ctx, conn.AgentID(), req.Target)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}

	targetConn, online := r.manager.GetConnection(req.Target)
	if online {
		notify := protocol.FriendRejectedNotify{FromAgent: conn.AgentID()}
		targetConn.Send(protocol.NewNotify(protocol.TypeFriendRejectedNotify, req.Target, notify))
	}

	conn.Send(protocol.NewResponse("friend.reject.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleFriendRemove(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendRemoveRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.friendshipService.Remove(ctx, conn.AgentID(), req.Target)
	if err != nil {
		code := protocol.ErrInternal
		if err == service.ErrNotFriends {
			code = protocol.ErrNotFriends
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	targetConn, online := r.manager.GetConnection(req.Target)
	if online {
		notify := protocol.FriendRemovedNotify{FromAgent: conn.AgentID()}
		targetConn.Send(protocol.NewNotify(protocol.TypeFriendRemovedNotify, req.Target, notify))
	}

	conn.Send(protocol.NewResponse("friend.remove.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleFriendBlock(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendBlockRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.friendshipService.Block(ctx, conn.AgentID(), req.Target)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("friend.block.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleFriendSetNickname(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendSetNicknameRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.friendshipService.SetNickname(ctx, conn.AgentID(), req.Target, req.Nickname)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("friend.set_nickname.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleFriendSetGroup(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendSetGroupRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.friendshipService.SetGroup(ctx, conn.AgentID(), req.Target, req.Group)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("friend.set_group.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleFriendSetTrust(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.FriendSetTrustRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.friendshipService.SetTrust(ctx, conn.AgentID(), req.Target, req.Level)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}
	conn.Send(protocol.NewResponse("friend.set_trust.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleConvStartDirect(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.ConvStartDirectRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	conv, msg, err := r.conversationService.StartDirect(ctx, conn.AgentID(), req.Target, req.FirstMessage)
	if err != nil {
		code := protocol.ErrInternal
		switch err {
		case service.ErrNotFriends:
			code = protocol.ErrNotFriends
		case service.ErrBlocked:
			code = protocol.ErrBlocked
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	targetConn, online := r.manager.GetConnection(req.Target)
	if online {
		unreadCount, _ := r.conversationService.GetUnreadCount(ctx, conv.ConversationID, req.Target)
		msgInfo := modelMsgToProtocol(msg)
		notify := protocol.MsgNewNotify{
			ConversationID: conv.ConversationID,
			Message:        msgInfo,
			UnreadCount:    unreadCount,
		}
		targetConn.Send(protocol.NewNotify(protocol.TypeMsgNewNotify, req.Target, notify))
	} else {
		r.messageService.EnqueueOffline(ctx, req.Target, msg.MessageID, conv.ConversationID)
	}

	conn.Send(protocol.NewResponse("conv.start_direct.resp", env.RequestID, conn.AgentID(), map[string]interface{}{
		"conversation_id": conv.ConversationID,
		"message_id":      msg.MessageID,
	}))
}

func (r *Router) handleConvCreateGroup(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.ConvCreateGroupRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	conv, firstMsg, err := r.conversationService.CreateGroup(ctx, conn.AgentID(), req.Name, req.InviteList, req.FirstMessage)
	if err != nil {
		code := protocol.ErrInternal
		if err == service.ErrNotFriends {
			code = protocol.ErrNotFriends
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	fromAgent, _ := r.agentService.GetByID(ctx, conn.AgentID())
	fromName := conn.AgentID()
	if fromAgent != nil {
		fromName = fromAgent.Name
	}

	activeMembers, _ := r.conversationService.ListActiveMembers(ctx, conv.ConversationID)
	memberIDs := make([]string, 0, len(activeMembers))
	for _, m := range activeMembers {
		memberIDs = append(memberIDs, m.AgentID)
	}

	for _, invitee := range req.InviteList {
		invitedNotify := protocol.ConvInvitedNotify{
			ConversationID: conv.ConversationID,
			Name:           req.Name,
			InvitedBy:      conn.AgentID(),
			InvitedByName:  fromName,
			Members:        memberIDs,
			CreatedAt:      conv.CreatedAt.Format(time.RFC3339),
		}

		targetConn, online := r.manager.GetConnection(invitee)
		if online {
			targetConn.Send(protocol.NewNotify(protocol.TypeConvInvitedNotify, invitee, invitedNotify))
			if firstMsg != nil {
				unreadCount, _ := r.conversationService.GetUnreadCount(ctx, conv.ConversationID, invitee)
				notify := protocol.MsgNewNotify{
					ConversationID: conv.ConversationID,
					Message:        modelMsgToProtocol(firstMsg),
					UnreadCount:    unreadCount,
				}
				targetConn.Send(protocol.NewNotify(protocol.TypeMsgNewNotify, invitee, notify))
			}
		} else if firstMsg != nil {
			r.messageService.EnqueueOffline(ctx, invitee, firstMsg.MessageID, conv.ConversationID)
		}
	}

	respData := map[string]interface{}{
		"conversation_id": conv.ConversationID,
		"name":            conv.Name,
	}
	if firstMsg != nil {
		respData["message_id"] = firstMsg.MessageID
	}
	conn.Send(protocol.NewResponse("conv.create_group.resp", env.RequestID, conn.AgentID(), respData))
}

func (r *Router) handleConvInvite(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.ConvInviteRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.conversationService.Invite(ctx, conn.AgentID(), req.ConversationID, req.Target)
	if err != nil {
		code := protocol.ErrInternal
		switch err {
		case service.ErrConvNotFound:
			code = protocol.ErrNotFound
		case service.ErrConvClosed:
			code = protocol.ErrConvClosed
		case service.ErrNotMember:
			code = protocol.ErrForbidden
		case service.ErrAlreadyMember:
			code = protocol.ErrConflict
		case service.ErrNotFriends:
			code = protocol.ErrNotFriends
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	fromAgent, _ := r.agentService.GetByID(ctx, conn.AgentID())
	fromName := conn.AgentID()
	if fromAgent != nil {
		fromName = fromAgent.Name
	}

	conv, _ := r.conversationService.GetByID(ctx, req.ConversationID)
	activeMembers, _ := r.conversationService.ListActiveMembers(ctx, req.ConversationID)
	memberIDs := make([]string, 0, len(activeMembers))
	for _, m := range activeMembers {
		memberIDs = append(memberIDs, m.AgentID)
	}

	invitedNotify := protocol.ConvInvitedNotify{
		ConversationID: req.ConversationID,
		Name:           conv.Name,
		InvitedBy:      conn.AgentID(),
		InvitedByName:  fromName,
		Members:        memberIDs,
		CreatedAt:      conv.CreatedAt.Format(time.RFC3339),
	}

	targetConn, online := r.manager.GetConnection(req.Target)
	if online {
		targetConn.Send(protocol.NewNotify(protocol.TypeConvInvitedNotify, req.Target, invitedNotify))
	}

	targetName := getAgentName(ctx, r.agentService, req.Target)
	joinedNotify := protocol.ConvMemberJoinedNotify{
		ConversationID: req.ConversationID,
		MemberAgentID:  req.Target,
		MemberName:     targetName,
		JoinedBy:       conn.AgentID(),
		JoinedByName:   fromName,
	}
	r.broadcastToConv(ctx, req.ConversationID, protocol.TypeConvMemberJoinedNotify, joinedNotify, conn.AgentID())

	conn.Send(protocol.NewResponse("conv.invite.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleConvLeave(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.ConvLeaveRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.conversationService.Leave(ctx, conn.AgentID(), req.ConversationID)
	if err != nil {
		code := protocol.ErrInternal
		switch err {
		case service.ErrConvNotFound:
			code = protocol.ErrNotFound
		case service.ErrConvClosed:
			code = protocol.ErrConvClosed
		case service.ErrNotMember:
			code = protocol.ErrForbidden
		case service.ErrCreatorCannotLeave:
			code = protocol.ErrNotCreator
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	agentName := getAgentName(ctx, r.agentService, conn.AgentID())
	notify := protocol.ConvMemberLeftNotify{
		ConversationID: req.ConversationID,
		MemberAgentID:  conn.AgentID(),
		MemberName:     agentName,
	}
	r.broadcastToConv(ctx, req.ConversationID, protocol.TypeConvMemberLeftNotify, notify, conn.AgentID())

	conn.Send(protocol.NewResponse("conv.leave.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleConvClose(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.ConvCloseRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	err := r.conversationService.Close(ctx, conn.AgentID(), req.ConversationID)
	if err != nil {
		code := protocol.ErrInternal
		switch err {
		case service.ErrConvNotFound:
			code = protocol.ErrNotFound
		case service.ErrConvClosed:
			code = protocol.ErrConvClosed
		case service.ErrNotMember:
			code = protocol.ErrForbidden
		case service.ErrNotCreator:
			code = protocol.ErrNotCreator
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	conv, _ := r.conversationService.GetByID(ctx, req.ConversationID)
	agentName := getAgentName(ctx, r.agentService, conn.AgentID())
	closedAt := ""
	if conv != nil && conv.ClosedAt != nil {
		closedAt = conv.ClosedAt.Format(time.RFC3339)
	}
	notify := protocol.ConvClosedNotify{
		ConversationID: req.ConversationID,
		ClosedBy:       conn.AgentID(),
		ClosedByName:   agentName,
		ClosedAt:       closedAt,
	}
	r.broadcastToConv(ctx, req.ConversationID, protocol.TypeConvClosedNotify, notify, conn.AgentID())

	conn.Send(protocol.NewResponse("conv.close.resp", env.RequestID, conn.AgentID(), map[string]interface{}{"ok": true}))
}

func (r *Router) handleMsgSend(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.MsgSendRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}

	var contentFile *model.ContentFile
	if req.ContentFile != nil {
		contentFile = &model.ContentFile{
			Filename:   req.ContentFile.Filename,
			Size:       req.ContentFile.Size,
			StorageKey: req.ContentFile.StorageKey,
		}
	}

	msg, err := r.messageService.Send(ctx, conn.AgentID(), req.ConversationID, req.MessageType, req.ContentText, contentFile, req.Mentions, req.ReplyTo)
	if err != nil {
		code := protocol.ErrInternal
		switch err {
		case service.ErrConvNotFound:
			code = protocol.ErrNotFound
		case service.ErrConvClosed:
			code = protocol.ErrConvClosed
		case service.ErrNotMember:
			code = protocol.ErrForbidden
		case service.ErrBlocked:
			code = protocol.ErrBlocked
		}
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), code, err.Error()))
		return
	}

	msgInfo := modelMsgToProtocol(msg)
	members, _ := r.conversationService.ListActiveMembers(ctx, req.ConversationID)
	for _, m := range members {
		if m.AgentID == conn.AgentID() {
			continue
		}

		unreadCount, _ := r.conversationService.GetUnreadCount(ctx, req.ConversationID, m.AgentID)
		notify := protocol.MsgNewNotify{
			ConversationID: req.ConversationID,
			Message:        msgInfo,
			UnreadCount:    unreadCount,
		}

		targetConn, online := r.manager.GetConnection(m.AgentID)
		if online {
			targetConn.Send(protocol.NewNotify(protocol.TypeMsgNewNotify, m.AgentID, notify))
		} else {
			r.messageService.EnqueueOffline(ctx, m.AgentID, msg.MessageID, req.ConversationID)
		}
	}

	conn.Send(protocol.NewResponse("msg.send.resp", env.RequestID, conn.AgentID(), map[string]interface{}{
		"message_id": msg.MessageID,
		"created_at": msg.CreatedAt.Format(time.RFC3339),
	}))
}

func (r *Router) handleDiscoverSearch(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.DiscoverSearchRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	agents, isFriendList, total, err := r.friendshipService.SearchWithFriendStatus(ctx, conn.AgentID(), req.Keyword, req.Skill, req.Tags, req.OnlineOnly, req.Limit, req.Offset)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}

	agentSummaries := make([]protocol.AgentSummary, 0)
	for i, agent := range agents {
		if agent.AgentID == conn.AgentID() {
			continue
		}
		caps, _ := agent.GetCapabilities()
		protoCaps := make([]protocol.Capability, 0, len(caps))
		for _, c := range caps {
			protoCaps = append(protoCaps, protocol.Capability{Skill: c.Skill, Tags: c.Tags})
		}
		agentSummaries = append(agentSummaries, protocol.AgentSummary{
			AgentID:      agent.AgentID,
			Name:         agent.Name,
			AvatarURL:    agent.AvatarURL,
			StatusText:   agent.StatusText,
			Online:       r.manager.IsOnline(agent.AgentID),
			IsFriend:     isFriendList[i],
			Capabilities: protoCaps,
		})
	}

	resp := protocol.DiscoverSearchResponse{
		Agents: agentSummaries,
		Total:  total,
	}
	conn.Send(protocol.NewResponse(protocol.TypeDiscoverSearchResp, env.RequestID, conn.AgentID(), resp))
}

func (r *Router) handleSyncConversations(ctx context.Context, conn *Connection, env protocol.Envelope) {
	convs, err := r.conversationService.ListByAgent(ctx, conn.AgentID(), "active")
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}

	convInfos := make([]protocol.ConversationInfo, 0, len(convs))
	for _, conv := range convs {
		members, _ := r.conversationService.ListMembers(ctx, conv.ConversationID)
		memberIDs := make([]string, 0, len(members))
		for _, m := range members {
			memberIDs = append(memberIDs, m.AgentID)
		}
		unreadCount, _ := r.conversationService.GetUnreadCount(ctx, conv.ConversationID, conn.AgentID())

		info := protocol.ConversationInfo{
			ConversationID:     conv.ConversationID,
			Type:               conv.Type,
			Name:               conv.Name,
			Status:             conv.Status,
			CreatedBy:          conv.CreatedBy,
			LastMessagePreview: conv.LastMessagePreview,
			UnreadCount:        unreadCount,
			Members:            memberIDs,
			CreatedAt:          conv.CreatedAt.Format(time.RFC3339),
		}
		if conv.LastMessageAt != nil {
			info.LastMessageAt = conv.LastMessageAt.Format(time.RFC3339)
		}
		if conv.ClosedAt != nil {
			info.ClosedAt = conv.ClosedAt.Format(time.RFC3339)
		}
		convInfos = append(convInfos, info)
	}

	resp := protocol.SyncConversationsResponse{Conversations: convInfos}
	conn.Send(protocol.NewResponse(protocol.TypeSyncConversationsResp, env.RequestID, conn.AgentID(), resp))
}

func (r *Router) handleSyncMessages(ctx context.Context, conn *Connection, env protocol.Envelope) {
	var req protocol.SyncMessagesRequest
	if !parseData(env.Data, &req) {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrBadRequest, "invalid request data"))
		return
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}

	isMember, err := r.conversationService.IsMember(ctx, req.ConversationID, conn.AgentID())
	if err != nil || !isMember {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrForbidden, "not a member"))
		return
	}

	messages, err := r.messageService.ListMessages(ctx, req.ConversationID, conn.AgentID(), req.AfterMessageID, req.Limit)
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}

	msgInfos := make([]protocol.MessageInfo, 0, len(messages))
	for _, msg := range messages {
		msgInfos = append(msgInfos, modelMsgToProtocol(msg))
	}

	resp := protocol.SyncMessagesResponse{
		ConversationID: req.ConversationID,
		Messages:       msgInfos,
	}
	conn.Send(protocol.NewResponse(protocol.TypeSyncMessagesResp, env.RequestID, conn.AgentID(), resp))
}

func (r *Router) handleSyncFriends(ctx context.Context, conn *Connection, env protocol.Envelope) {
	friendships, err := r.friendshipService.ListFriends(ctx, conn.AgentID())
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}

	friendInfos := make([]protocol.FriendInfo, 0, len(friendships))
	for _, f := range friendships {
		friendID, nickname, group, trustLevel, _ := f.GetFriendSide(conn.AgentID())
		agent, _ := r.agentService.GetByID(ctx, friendID)
		if agent == nil {
			continue
		}
		caps, _ := agent.GetCapabilities()
		protoCaps := make([]protocol.Capability, 0, len(caps))
		for _, c := range caps {
			protoCaps = append(protoCaps, protocol.Capability{Skill: c.Skill, Tags: c.Tags})
		}

		friendInfos = append(friendInfos, protocol.FriendInfo{
			AgentID:      friendID,
			Name:         agent.Name,
			AvatarURL:    agent.AvatarURL,
			StatusText:   agent.StatusText,
			Online:       r.manager.IsOnline(friendID),
			Nickname:     nickname,
			Group:        group,
			TrustLevel:   trustLevel,
			Capabilities: protoCaps,
		})
	}

	resp := protocol.SyncFriendsResponse{Friends: friendInfos}
	conn.Send(protocol.NewResponse(protocol.TypeSyncFriendsResp, env.RequestID, conn.AgentID(), resp))
}

func (r *Router) handleSyncFriendRequests(ctx context.Context, conn *Connection, env protocol.Envelope) {
	requests, err := r.friendshipService.ListReceivedRequests(ctx, conn.AgentID())
	if err != nil {
		conn.Send(protocol.NewError("error", env.RequestID, conn.AgentID(), protocol.ErrInternal, err.Error()))
		return
	}

	reqInfos := make([]protocol.FriendRequestInfo, 0, len(requests))
	for _, fr := range requests {
		fromAgent := fr.InitiatedBy
		agent, _ := r.agentService.GetByID(ctx, fromAgent)
		name := fromAgent
		if agent != nil {
			name = agent.Name
		}
		reqInfos = append(reqInfos, protocol.FriendRequestInfo{
			FromAgent: fromAgent,
			Name:      name,
			Message:   fr.RequestMessage,
			CreatedAt: fr.CreatedAt.Format(time.RFC3339),
		})
	}

	resp := protocol.SyncFriendRequestsResponse{Requests: reqInfos}
	conn.Send(protocol.NewResponse(protocol.TypeSyncFriendRequestsResp, env.RequestID, conn.AgentID(), resp))
}

// ============================================================
// Helper functions
// ============================================================

func parseData(data interface{}, target interface{}) bool {
	if data == nil {
		return false
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return false
	}
	err = json.Unmarshal(jsonBytes, target)
	return err == nil
}

func modelMsgToProtocol(msg *model.Message) protocol.MessageInfo {
	info := protocol.MessageInfo{
		MessageID:      msg.MessageID,
		ConversationID: msg.ConversationID,
		FromAgent:      msg.FromAgent,
		MessageType:    msg.MessageType,
		ContentText:    msg.ContentText,
		Mentions:       msg.Mentions,
		ReplyTo:        msg.ReplyTo,
		SystemEvent:    msg.SystemEvent,
		CreatedAt:      msg.CreatedAt.Format(time.RFC3339),
	}
	if msg.ContentFile != nil && len(msg.ContentFile) > 0 {
		var cf protocol.FileInfo
		json.Unmarshal(msg.ContentFile, &cf)
		info.ContentFile = &cf
	}
	return info
}

func getAgentName(ctx context.Context, agentService *service.AgentService, agentID string) string {
	agent, err := agentService.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return agentID
	}
	return agent.Name
}

func (r *Router) broadcastToConv(ctx context.Context, conversationID string, msgType string, data interface{}, excludeAgentID string) {
	members, err := r.conversationService.ListActiveMembers(ctx, conversationID)
	if err != nil {
		return
	}
	for _, m := range members {
		if m.AgentID == excludeAgentID {
			continue
		}
		r.manager.SendToAgent(m.AgentID, protocol.NewNotify(msgType, m.AgentID, data))
	}
}
