import React, { useEffect } from "react"
import { Route, Switch, useHistory } from "react-router"
import { getCourseID, isTeacher } from "../Helpers"
import { useActions, useAppState } from "../overmind"
import Card from "../components/Card"
import CourseBanner from "../components/CourseBanner"
import GroupPage from "./GroupPage"
import Members from "../components/Members"
import RedirectButton from "../components/RedirectButton"
import Results from "../components/Results"
import ReviewPage from "../components/ReviewPage"
import StatisticsView from "../components/Statistics"
import Assignments from "../components/teacher/Assignments"
import Alert from "../components/Alert"

/* */
const TeacherPage = (): JSX.Element => {
    const state = useAppState()
    const courseID = getCourseID()
    const isAuthorizedTeacher = useActions().isAuthorizedTeacher
    const history = useHistory()
    const root = `/course/${courseID}`

    const members = {title: "View Members", notification: state.pendingEnrollments.length > 0 ? {color: "warning", text: "Pending enrollments"} : undefined, text: "View all students, and approve new enrollments.", buttonText: "Members", to: `${root}/members`}
    const results = {title: "View results", text: "View results for all students in the course.", buttonText: "Results", to: `${root}/results`}
    const groups = {title: "Manage Groups", notification: state.pendingGroups.length > 0 ? {color: "warning", text: "Pending groups"} : undefined, text: "View, edit or delete course groups.", buttonText: "Groups", to: `${root}/groups`}
    const statistics = {title: "Statistics", text: "See statistics for the course.", buttonText: "Statistics", to: `${root}/statistics`}
    const assignments = {title: "Manage Assignments", text: "View and edit assignments.", buttonText: "Assignments", to: `${root}/assignments`}
  

    useEffect(() => {
        // Redirect to OAuth authorization if user is teacher but not authorized
        if (isTeacher(state.enrollmentsByCourseId[courseID])) {
            isAuthorizedTeacher().then(authorized => {
                if (!authorized) {
                    window.location.assign("auth/github-teacher")
                }
            })
        }
    }, [])


    return (
        <div>
            <RedirectButton to={root}></RedirectButton>
            <CourseBanner />
            <Alert />
            <div className="row" hidden={history.location.pathname != root}>
                <Card {...results}></Card>
                <Card {...groups}></Card>
                <Card {...members}></Card>
                <Card {...statistics}></Card>
                <Card {...assignments}></Card>
            </div>
            <Switch>
                <Route path={`/course/:id/groups`} exact component={GroupPage}></Route>
                <Route path={"/course/:id/members"} component={Members}></Route>
                <Route path={"/course/:id/review"} component={ReviewPage}></Route>
                <Route path={"/course/:id/results"} component={Results}></Route>
                <Route path={"/course/:id/statistics"} component={StatisticsView}></Route>
                <Route path={"/course/:id/assignments"} component={Assignments}></Route>
            </Switch>
        </div>
    )
}

export default TeacherPage