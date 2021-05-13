import React, { useEffect } from 'react'
import { useParams } from 'react-router'
import { Assignment, Submission } from '../../proto/ag_pb'
import { useOvermind } from '../overmind'
import CourseUtilityLinks from './CourseUtilityLinks'
import LabResultTable from './LabResultTable'
import ReviewResult from './ReviewResult'

interface MatchProps {
    id: string
    lab: string
}

interface TeacherLab {
    submissionID: number
    assignmentID: number
}

/** Displays a Lab submission based on the /course/:id/:lab route
 *  
 *  If used to display a lab for grading purposes, pass in a TeacherLab object
 */
const Lab = (teacher: TeacherLab) => {
    const { state, actions } = useOvermind()
    const {id ,lab} = useParams<MatchProps>()
    const courseID = Number(id)
    const assignmentID = Number(lab)
    const teacherLab = teacher.assignmentID && teacher.submissionID
    useEffect(() => {
        // Do not start the commit hash fetch-loop for submissions that are not personal
        if (!teacherLab) {
            actions.setActiveLab(assignmentID)

            const ping = setInterval(() => {  
                actions.getSubmissionCommitHash({courseID: courseID, assignmentID: assignmentID})
            }, 5000)

            return () => {clearInterval(ping), actions.setActiveLab(-1)}
        }
    }, [lab])

    
    const Lab = () => {
        let submission: Submission | undefined
        let assignment: Assignment | undefined

        // If used for grading purposes, retrieve submission from courseSubmissions
        if (teacherLab) {
            state.courseSubmissions[courseID].forEach(link => {
                link.getSubmissionsList().forEach(s => {
                    if (s.getSubmission()?.getId() === teacher.submissionID) {
                        submission = s.getSubmission()
                    }
                })
            });
            assignment = state.assignments[courseID].find(a => a.getId() === teacher.assignmentID)
        } 
        // Retreive personal submission
        else {
            submission = state.submissions[courseID]?.find(s => s.getAssignmentid() === assignmentID)
            assignment = state.assignments[courseID]?.find(a => a.getId() === assignmentID)
        }
        
        // Confirm both assignment and submission exists before attempting to render
        if (assignment && submission) {
            let buildLog = ""
            if (submission.getBuildinfo()) {
                const buildInfo = JSON.parse(submission.getBuildinfo())
                // Prettifies the build log. (credit to nicro950 for this snippet)
                buildLog = buildInfo.buildlog.split("\n").map((x: string, i: number) => <span key={i} >{x}<br /></span>);
            }
            return (
                <div key={submission.getId()}>

                    <LabResultTable submission={submission} assignment={assignment} />

                    {assignment.getSkiptests() && submission.getReviewsList().length > 0 ? 
                    <ReviewResult review={submission.getReviewsList()}/> 
                    : ""}

                    <div className="card bg-light">
                        <code className="card-body" style={{color: "#c7254e"}}>{buildLog}</code>
                    </div>
                </div>
            )
        }
        return (
            <div>No submission found</div>
        )
    }

    return (
        <div className="box row">
            <div className={teacherLab ? "" : "col-md-9"}>
                <Lab />
            </div>
            {!teacherLab ? <CourseUtilityLinks courseID={courseID} />
            : "" }
        </div>
    )
}

export default Lab