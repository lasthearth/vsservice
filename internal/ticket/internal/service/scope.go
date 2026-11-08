package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// ManageScope is what the staff side needs. Grant it to the Logto "admin"
// role together with the other admin permissions.
const ManageScope interceptor.Scope = "tickets:manage"

// Scope implements interceptor.Scoper. The player methods only need a signed-in
// caller: ownership is checked on the ticket, and someone else's ticket answers
// NOT_FOUND.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/ticket.v1.TicketService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "CreateTicket"):        interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ListMyTickets"):       interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetTicket"):           interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "PostTicketMessage"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "CloseTicket"):         interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ReopenTicket"):        interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetMyTicketCounters"): interceptor.ScopeAuthenticated,

		interceptor.Method(srvName + "AdminListTickets"):       ManageScope,
		interceptor.Method(srvName + "AdminGetTicket"):         ManageScope,
		interceptor.Method(srvName + "AdminReplyTicket"):       ManageScope,
		interceptor.Method(srvName + "AdminUpdateTicket"):      ManageScope,
		interceptor.Method(srvName + "AdminGetTicketCounters"): ManageScope,
		interceptor.Method(srvName + "ListSavedReplies"):       ManageScope,
		interceptor.Method(srvName + "CreateSavedReply"):       ManageScope,
		interceptor.Method(srvName + "UpdateSavedReply"):       ManageScope,
		interceptor.Method(srvName + "DeleteSavedReply"):       ManageScope,
	}
}
