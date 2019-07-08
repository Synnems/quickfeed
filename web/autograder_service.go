package web

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/autograde/aguis/ag"
	"github.com/autograde/aguis/database"
	"github.com/autograde/aguis/web/auth"
)

// AutograderService holds references to the database and
// other shared data structures.
type AutograderService struct {
	logger *zap.SugaredLogger
	db     *database.GormDB
	scms   *auth.Scms
	bh     BaseHookOptions
}

// NewAutograderService returns an AutograderService object.
func NewAutograderService(logger *zap.Logger, db *database.GormDB, scms *auth.Scms, bh BaseHookOptions) *AutograderService {
	return &AutograderService{
		logger: logger.Sugar(),
		db:     db,
		scms:   scms,
		bh:     bh,
	}
}

// GetRepositoryURL returns a repository URL for the requested repository type.
func (s *AutograderService) GetRepositoryURL(ctx context.Context, in *pb.RepositoryRequest) (*pb.URLResponse, error) {
	currentUser, err := s.getCurrentUser(ctx)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get current user")
	}
	repoURL, err := s.getRepositoryURL(currentUser, in)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to fetch repository URL")
	}
	return repoURL, nil
}

// GetUser returns user information for the given user, excluding remote identities.
func (s *AutograderService) GetUser(ctx context.Context, in *pb.RecordRequest) (*pb.User, error) {
	if !s.hasAccess(ctx, in.ID) {
		return nil, status.Errorf(codes.PermissionDenied, "only admin can access another user")
	}
	usr, err := s.getUser(in)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get user")
	}
	usr.RemoveRemoteID()
	return usr, nil
}

// GetUsers returns a list of all users.
// Frontend note: This method is used from AdminPage.tsx:users():35.
func (s *AutograderService) GetUsers(ctx context.Context, in *pb.Void) (*pb.Users, error) {
	if !s.isAdmin(ctx) {
		return nil, status.Errorf(codes.PermissionDenied, "only admin can access other users")
	}
	usrs, err := s.getUsers()
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get users")
	}
	usrs.RemoveRemoteIDs()
	return usrs, nil
}

// UpdateUser updates the current users's information and returns the updated user.
// Admin users can update other users information, whereas non-admin users can only
// update their own information.
func (s *AutograderService) UpdateUser(ctx context.Context, in *pb.User) (*pb.User, error) {
	if !s.hasAccess(ctx, in.ID) {
		return nil, status.Errorf(codes.PermissionDenied, "only admin can access another user")
	}
	usr, err := s.updateUser(s.isAdmin(ctx), in)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to update current user")
	}
	usr.RemoveRemoteID()
	return usr, nil
}

// IsAuthorizedTeacher checks whether current user has teacher scopes
func (s *AutograderService) IsAuthorizedTeacher(ctx context.Context, in *pb.Void) (*pb.AuthorizationResponse, error) {
	// TODO(vera): upgrade to send provider from client. Currently not supported for other providers anyway
	// Hein @Vera: it may be easier to pass along the courseID from the client as is done for UpdateEnrollment (see below)
	_, scm, err := s.getUserAndSCM(ctx, "github", true)
	if err != nil {
		return nil, err
	}
	return &pb.AuthorizationResponse{
		IsAuthorized: s.hasTeacherScopes(ctx, scm),
	}, nil
}

// CreateCourse creates a new course.
// Only users with admin role can create new courses.
func (s *AutograderService) CreateCourse(ctx context.Context, in *pb.Course) (*pb.Course, error) {
	usr, scm, err := s.getUserAndSCM(ctx, in.Provider, true)
	if err != nil {
		return nil, err
	}

	// make sure that the current user is set as course creator
	in.CourseCreatorID = usr.GetID()
	course, err := NewCourse(ctx, in, s.db, scm, s.bh)
	if err != nil {
		s.logger.Error(err)
		if err == ErrAlreadyExists {
			return nil, status.Errorf(codes.AlreadyExists, err.Error())
		}
		return nil, status.Errorf(codes.InvalidArgument, "failed to create course")
	}
	return course, nil
}

// UpdateCourse changes the course information details.
// Only users with teacher role (admin) can update the course details.
func (s *AutograderService) UpdateCourse(ctx context.Context, in *pb.Course) (*pb.Void, error) {
	_, scm, err := s.getUserAndSCM(ctx, in.Provider, true)
	if err != nil {
		return nil, err
	}

	if err = UpdateCourse(ctx, in, s.db, scm); err != nil {
		s.logger.Error(err)
		err = status.Errorf(codes.InvalidArgument, "failed to update course")
	}
	return &pb.Void{}, err
}

// GetCourse returns course information for the given course.
func (s *AutograderService) GetCourse(ctx context.Context, in *pb.RecordRequest) (*pb.Course, error) {
	course, err := s.getCourse(in.GetID())
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "course not found")
	}
	return course, nil
}

// GetCourses returns a list of all courses.
func (s *AutograderService) GetCourses(ctx context.Context, in *pb.Void) (*pb.Courses, error) {
	return ListCourses(s.db)
}

// CreateEnrollment enrolls a new student for the course specified in the request.
func (s *AutograderService) CreateEnrollment(ctx context.Context, in *pb.Enrollment) (*pb.Void, error) {
	return &pb.Void{}, CreateEnrollment(in, s.db)
}

// UpdateEnrollment updates the enrollment status of a student as specified in the request.
func (s *AutograderService) UpdateEnrollment(ctx context.Context, in *pb.Enrollment) (*pb.Void, error) {
	// must be admin to update enrollment status
	_, scm, err := s.getUserAndSCM2(ctx, in.GetCourseID(), true)
	if err != nil {
		return nil, err
	}
	return &pb.Void{}, UpdateEnrollment(ctx, in, s.db, scm)
}

// GetCoursesWithEnrollment returns all courses with enrollments of the type specified in the request.
func (s *AutograderService) GetCoursesWithEnrollment(ctx context.Context, in *pb.RecordRequest) (*pb.Courses, error) {
	//TODO(meling) these direct calls and returns needs to be logged here and return status.Error instead
	return ListCoursesWithEnrollment(in, s.db)
}

// GetAssignments returns a list of all assignments.
func (s *AutograderService) GetAssignments(ctx context.Context, in *pb.RecordRequest) (*pb.Assignments, error) {
	return ListAssignments(in, s.db)
}

// GetEnrollmentsByCourse returns all enrollments for the course specified in the request.
func (s *AutograderService) GetEnrollmentsByCourse(ctx context.Context, in *pb.EnrollmentRequest) (*pb.Enrollments, error) {
	enrolls, err := GetEnrollmentsByCourse(in, s.db)
	if err != nil {
		return nil, err
	}
	enrolls.RemoveRemoteIDs()
	return enrolls, nil
}

// GetGroup returns information about a group.
func (s *AutograderService) GetGroup(ctx context.Context, in *pb.RecordRequest) (*pb.Group, error) {
	group, err := s.getGroup(in)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get group")
	}
	usr, err := s.getCurrentUser(ctx)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get current user")
	}
	if !(s.isTeacher(usr.ID, group.GetCourseID()) || s.hasAccessG(ctx, group.GetUsers())) {
		return nil, status.Errorf(codes.PermissionDenied, "only members, teachers or admin can access a group")
	}
	group.RemoveRemoteIDs()
	return group, nil
}

// GetGroups returns a list of groups created for the course.
func (s *AutograderService) GetGroups(ctx context.Context, in *pb.RecordRequest) (*pb.Groups, error) {
	if !s.isAdmin(ctx) {
		return nil, status.Errorf(codes.PermissionDenied, "only admin can access other groups")
	}
	groups, err := s.getGroups(in)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get groups")
	}
	groups.RemoveRemoteIDs()
	return groups, nil
}

// GetGroupByUserAndCourse returns the group of the given student for a given course.
func (s *AutograderService) GetGroupByUserAndCourse(ctx context.Context, in *pb.ActionRequest) (*pb.Group, error) {
	if !s.hasAccess(ctx, in.UserID) {
		return nil, status.Errorf(codes.PermissionDenied, "only admin can access another group")
	}
	group, err := s.getGroupByUserAndCourse(in)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get group for given user and course")
	}
	group.RemoveRemoteIDs()
	return group, nil
}

// CreateGroup creates a new group.
func (s *AutograderService) CreateGroup(ctx context.Context, in *pb.Group) (*pb.Group, error) {
	usr, err := s.getCurrentUser(ctx)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get current user")
	}
	group, err := s.createGroup(in, usr)
	if err != nil {
		s.logger.Error(err)
		if _, ok := status.FromError(err); !ok {
			// set err to generic error for the frontend
			err = status.Error(codes.Internal, "server error; check server logs for details")
		}
		return nil, err
	}
	group.RemoveRemoteIDs()
	return group, nil
}

// UpdateGroup updates group information.
func (s *AutograderService) UpdateGroup(ctx context.Context, in *pb.Group) (*pb.Void, error) {
	// need not be admin to approve or update group composition
	usr, scm, err := s.getUserAndSCM2(ctx, in.GetCourseID(), false)
	if err != nil {
		return nil, err
	}
	if !s.isTeacher(usr.ID, in.GetCourseID()) {
		return nil, status.Errorf(codes.PermissionDenied, "only teachers can update groups")
	}

	err = s.updateGroup(ctx, in, usr, scm)
	if err != nil {
		s.logger.Error(err)
		if _, ok := status.FromError(err); !ok {
			// set err to generic error for the frontend
			err = status.Error(codes.Internal, "server error; check server logs for details")
		}
	}
	return &pb.Void{}, err
}

// DeleteGroup removes group record from the database
func (s *AutograderService) DeleteGroup(ctx context.Context, in *pb.Group) (*pb.Void, error) {
	//TODO(meling) This will call IsValid() method on Group also, which would probably not pass for this request
	// Easiest is perhaps to switch it with a simple RecordRequest with checking just the ID.
	if in.GetID() < 1 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid payload")
	}
	return &pb.Void{}, s.deleteGroup(in)
}

// GetSubmission returns a student submission
func (s *AutograderService) GetSubmission(ctx context.Context, in *pb.RecordRequest) (*pb.Submission, error) {
	usr, err := s.getCurrentUser(ctx)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "failed to get current user")
	}
	return GetSubmission(in, s.db, usr)
}

// GetSubmissions returns the submissions matching the query encoded in the action request.
func (s *AutograderService) GetSubmissions(ctx context.Context, in *pb.ActionRequest) (*pb.Submissions, error) {
	if !s.hasGroupAccess(ctx, in.GetCourseID(), in.GetUserID(), in.GetGroupID()) {
		return nil, status.Errorf(codes.PermissionDenied, "only members, teachers or admin can access submissions")
	}
	submissions, err := s.getSubmissions(in)
	if err != nil {
		s.logger.Error(err)
		return nil, status.Errorf(codes.NotFound, "no submissions found")
	}
	return submissions, nil
}

// UpdateSubmission changes submission information
func (s *AutograderService) UpdateSubmission(ctx context.Context, in *pb.RecordRequest) (*pb.Void, error) {
	//TODO(meling) UpdateSubmission requires administrator/teacher access
	return &pb.Void{}, UpdateSubmission(in, s.db)
}

// RefreshCourse returns latest information about the course
func (s *AutograderService) RefreshCourse(ctx context.Context, in *pb.RecordRequest) (*pb.Assignments, error) {
	// must be admin to refresh course
	usr, scm, err := s.getUserAndSCM2(ctx, in.GetID(), true)
	if err != nil {
		return nil, err
	}
	return RefreshCourse(ctx, in, scm, s.db, usr)
}

// GetProviders returns a list of providers
func (s *AutograderService) GetProviders(ctx context.Context, in *pb.Void) (*pb.Providers, error) {
	providers := auth.GetProviders()
	if len(providers.GetProviders()) < 1 {
		s.logger.Error("found no enabled SCM providers")
		return nil, status.Errorf(codes.NotFound, "found no enabled SCM providers")
	}
	return providers, nil
}

// GetOrganizations returns a list of directories for a course
func (s *AutograderService) GetOrganizations(ctx context.Context, in *pb.ActionRequest) (*pb.Organizations, error) {
	ctx, cancel := context.WithTimeout(ctx, MaxWait)
	defer cancel()

	_, scm, err := s.getUserAndSCM(ctx, in.Provider, false)
	if err != nil {
		return nil, err
	}
	return ListOrganizations(ctx, s.db, scm)
}

// GetRepository is not yet implemented
func (s *AutograderService) GetRepository(ctx context.Context, in *pb.RepositoryRequest) (*pb.Repository, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}